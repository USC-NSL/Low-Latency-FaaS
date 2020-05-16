
# This is the P4 switch controller that works for the P4 switch in a 
# FaaS-NFV system. The controller sets up the collection of forwarding 
# rules for each new flow.
# (1) Upon receiving the first packet of a flow, the switch controller
# queries FaaS-Controller to select all potentially related container
# instances, and encodes results in the NSH header.
# (2) Upon receiving all subsequent packets of a flow, the switch ensures
# that these packets go through the same path.

import time
import os
import sys
import threading
import cmd
import signal
from scapy.all import *
import argparse
import logging
import collections
import copy
import unicodedata

# Adds the directory where the generated PD file is located.
parser = argparse.ArgumentParser()
parser.add_argument( \
    '--install-dir', required=False, help='path to install directory', \
    default='/root/bf-sde-8.2.0/install/', type=str)
args = parser.parse_args()
install_path = os.path.join(args.install_dir, 'lib/python2.7/site-packages')
sys.path.append(install_path)
sys.path.append(os.path.join(install_path, 'tofino'))
sys.path.append(os.path.join(install_path, 'tofinopd'))

import faas_switch_mac.p4_pd_rpc.faas_switch_mac as pd_rpc
from faas_switch_mac.p4_pd_rpc.ttypes import *
import pal_rpc.pal as pal_rpc
from pal_rpc.ttypes import *
import conn_mgr_pd_rpc.conn_mgr as conn_mgr_rpc
from res_pd_rpc.ttypes import *
from mc_pd_rpc.ttypes import *
from ptf.thriftutils import *
from thrift.transport import TSocket
from thrift.transport import TTransport
from thrift.protocol import TBinaryProtocol
from thrift.protocol import TMultiplexedProtocol
# Protobuf and GRPC.
import grpc
from concurrent import futures
from google.protobuf.empty_pb2 import Empty
import protobuf.message_pb2 as message_pb
import protobuf.switch_service_pb2 as switch_pb
import protobuf.switch_service_pb2_grpc as switch_rpc
import protobuf.faas_service_pb2 as faas_pb
import protobuf.faas_service_pb2_grpc as faas_rpc


# Switch ports for two example workers.
# Todo(Jianfeng): remove these hardcoded IPs.
kSwitchServerAddress = "[::]:10516"
kFaaSServerAddress = "204.57.3.169:10515"
# 36 is the Netronome port.
kSwitchPortTraffic = 36
kSwitchPortWorker1 = 20
kSwitchPortWorker2 = 132

def int_to_bytes(n, length, endianess='big'):
    h = '%x' % n
    s = ('0'*(len(h) % 2) + h).zfill(length*2).decode('hex')
    return s if endianess == 'big' else s[::-1]

# This function encodes the |flow_id| in the nsh.context field.
# The encoding format is described as following:
# |flow_id| is a 32-bit integer. |nsh.context| is a 128-bit packet
# header field, which consists of |00.000| + |flow_id|. |flow_id|
# presents as a raw bytes. For example, 0x00000001 is 
# '\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01'.
def nsh_context_encoder(flow_id):
    context = flow_id & 0xFFFFFFFF
    return int_to_bytes(context, 16)


# This class implements a thrift PD interface that talks with the
# switch daemon.
class ThriftInterface(object):
    DEVICE = 0
    PIPE = 0xFFFF
    DEVICE_TGT = DevTarget_t(0, hex_to_i16(PIPE))

    _transport = None
    _transport_diag = None
    _protocol = None
    _conn_mgr_protocol = None
    _pal_protocol = None
    _switch_p4_protocol = None

    client = None
    pal_client = None
    conn_mgr = None
    # |sess_hdl| is the handler that must be provided when making RPC calls.
    sess_hdl = None

    _dev_ports = {}

    def __init__(self):
        self._transport = TSocket.TSocket('localhost', 9090)
        self._transport_diag = TTransport.TBufferedTransport(self._transport)
        self._protocol = TBinaryProtocol.TBinaryProtocol(self._transport_diag)

        self._conn_mgr_protocol = TMultiplexedProtocol.TMultiplexedProtocol( \
            self._protocol, 'conn_mgr')
        self.conn_mgr = conn_mgr_rpc.Client(self._conn_mgr_protocol)

        self._pal_protocol = TMultiplexedProtocol.TMultiplexedProtocol( \
            self._protocol, 'pal')
        self.pal_client = pal_rpc.Client(self._pal_protocol)

        self._switch_p4_protocol = TMultiplexedProtocol.TMultiplexedProtocol( \
            self._protocol, 'faas_switch_mac')        
        self.client = pd_rpc.Client(self._switch_p4_protocol)

        # Starts the switch daemon RPC connection.
        self._transport_diag.open()

        # |self.sess_hdl| is the session handler that works for all RPCs.
        self.sess_hdl = self.conn_mgr.client_init()

        # |_dev_ports| records all related switch ports.
        self._dev_ports = { \
            '19/0': kSwitchPortWorker1, \
            '1/0': kSwitchPortWorker2, \
            '21/0': kSwitchPortTraffic,}

    def cleanup(self):
        # Close the thrift connection.
        if self.sess_hdl:
            print "closing session"
            status = self.conn_mgr.client_cleanup(self.sess_hdl)
        if self._transport_diag:
            self._transport_diag.close()
        if self._transport:
            self._transport.close()

    # Returns the number of entries in the table with |table_name|.
    def dump_table(self, table_name):
        command = 'self.client.%s_get_entry_count' %(table_name)
        # get the number of entries
        num_entries = eval(command) (self.sess_hdl, self.DEVICE_TGT)
        return num_entries

    # Inserts one rule into the table with |table_name|.
    # |action| is a string, and represents the action's name.
    # |match_spec| must match the table's RPC spec_t definition.
    # |action_spec| must match the action's RPC spec_t definition.
    # |action_spec| may be None.
    def insert_exact_match_rule(self, table_name, action, match_spec, action_spec):
        command = 'self.client.%s_table_add_with_%s' %(table_name, action)
        if action_spec:
            response = eval(command) (self.sess_hdl, self.DEVICE_TGT, match_spec, action_spec)
        else:
            response = eval(command) (self.sess_hdl, self.DEVICE_TGT, match_spec)
        return response

    def delete_exact_match_rule(self, table_name, entry_handler):
        command = 'self.client.%s_table_delete' %(table_name)
        response = eval(command) (self.sess_hdl, self.DEVICE, entry_handler)
        return response

    # |port_channel| is a string in the form of 'port/channel'.
    # e.g. '1/0' represents the port #1 with channel #0.
    # |sync_mode| is an integer (0, 1, 2) that represents the sync mode for the link.
    def setup_dev_port(self, port_channel, device_port, sync_mode):
        self._dev_ports[port_channel] = device_port
        if sync_mode not in (0, 1, 2):
            return

        self.delete_dev_port(port_channel)

        command_add = 'self.pal_client.pal_port_add'
        eval(command_add) (self.DEVICE, device_port, pal_port_speed_t.BF_SPEED_40G, pal_fec_type_t.BF_FEC_TYP_NONE)

        if sync_mode == 2:
            command_an = 'self.pal_client.pal_port_an_set'
            eval(command_an) (self.DEVICE, device_port, sync_mode)

        command_enb = 'self.pal_client.pal_port_enable'
        eval(command_enb) (self.DEVICE, device_port)

        if sync_mode != 1:
            command_an = 'self.pal_client.pal_port_an_set'
            eval(command_an) (self.DEVICE, device_port, sync_mode)
        return

    # |port_channel| is a string in the form of 'port/channel'.
    def delete_dev_port(self, port_channel):
        if port_channel not in self._dev_ports:
            return

        device_port = self._dev_ports[port_channel]
        command_del = 'self.pal_client.pal_port_del'
        eval(command_del) (self.DEVICE, device_port)
        return

    def read_digests(self):
        digests = eval('self.client.%s') (self.sess_hdl)
        return digests


# This class implements the abstract switch table that manages table entries
# in a real switch table.
# faas_port_table:
# (ingress port) -> (egress port)
#
# faas_conn_table:
# (ip.src) (ip.dst) (ip.protocol) (tcp.srcPort) (tcp.dstPort) -> (spi, si, context)
#
# faas_instance_table:
# (spi) (si) -> (switchPort)
class SwitchTable(object):
    _name = None
    _table_entries = {}
    _actions = set()

    def __init__(self, table_name):
        self._name = table_name

    def add_action(self, action):
        self._actions.add(action)

    def has_action(self, action):
        return action in self._actions

    def get_match_action_spec(self, action, args):
        #print action, args, self._name
        if action not in self._actions:
            return None, None, None

        match_spec = None
        action_spec = None
        entry_key = None

        # Handles different switch tables.
        if self._name == 'faas_port_table':
            if len(args) < 2:
                return None, None, None

            ingress_port = int(args[0])
            match_spec = faas_switch_mac_faas_port_table_match_spec_t( \
                ig_intr_md_ingress_port=ingress_port, \
                )
            entry_key = (ingress_port)

            action_args = args[1:]
            if action == 'faas_port_table_hit':
                if len(action_args) != 1:
                    return None, None, None

                egress_port = int(action_args[0])
                # API: (faas_port_table)_table_add_with_(faas_port_table_hit)
                action_spec = faas_switch_mac_faas_port_table_hit_action_spec_t( \
                    action_egress_port=egress_port, \
                    )
            elif action == 'faas_port_table_miss':
                # API: (faas_port_table)_table_add_with_(faas_port_table_miss)
                pass
        elif self._name == 'faas_conn_table':
            if len(args) < 5:
                return None, None, None

            src_ip = args[0]
            # src = int(socket.inet_aton(src_ip).encode('hex'), 16)
            src = struct.unpack("!i", socket.inet_aton(src_ip))[0]
            dst_ip = args[1]
            # dst = int(socket.inet_aton(dst_ip).encode('hex'), 16)
            dst = struct.unpack("!i", socket.inet_aton(dst_ip))[0]
            protocol = int(args[2])
            # ipv4.protocol: TCP 0x06, UDP 0x11
            if protocol not in (0x6, 0x11):
                return None, None, None
            sport = int(args[3])
            dport = int(args[4])

            match_spec = faas_switch_mac_faas_conn_table_match_spec_t( \
                ipv4_srcAddr=src, \
                ipv4_dstAddr=dst, \
                ipv4_protocol=protocol, \
                tcp_srcPort=sport, \
                tcp_dstPort=dport,\
                )
            entry_key = (src_ip, dst_ip, protocol, sport, dport)

            action_args = args[5:]
            if action == 'faas_conn_table_hit':
                if len(action_args) != 2:
                    return None, None

                # switch_port: 16-bit int;
                # dest_mac: string;
                switch_port, dest_mac = int(action_args[0]), action_args[1]
                # API: (faas_conn_table)_table_add_with_(faas_conn_table_hit)
                action_spec = faas_switch_mac_faas_conn_table_hit_action_spec_t( \
                    action_switch_port=switch_port, action_dest_mac=dest_mac)
            elif action == 'faas_conn_table_miss':
                # API: (faas_conn_table)_table_add_with_(faas_conn_table_miss)
                pass
        elif self._name == 'faas_instance_table':
            if len(args) < 2:
                return None, None, None

            spi, si = int(args[0]), int(args[1])
            match_spec = faas_switch_faas_instance_table_match_spec_t( \
                nsh_spi=spi, \
                nsh_si=si, \
                )
            entry_key = (spi, si)

            action_args = args[2:]
            if action == 'faas_instance_table_hit':
                if len(action_args) != 1:
                    return None, None, None

                egress_port = int(action_args[0])
                # API: (faas_instance_table)_table_add_with_(faas_instance_table_hit)
                action_spec = faas_switch_mac_faas_instance_table_hit_action_spec_t( \
                    action_switchPort=egress_port, \
                    )
            elif action == 'faas_instance_table_hit_egress':
                if len(action_args) != 1:
                    return None, None, None

                egress_port = int(action_args[0])
                # API: (faas_instance_table)_table_add_with_(faas_instance_table_hit_egress)
                action_spec = faas_switch_mac_faas_instance_table_hit_egress_action_spec_t( \
                    action_switchPort=1, \
                    )
            elif action == 'faas_instance_table_miss':
                # API: (faas_instance_table)_table_add_with_(faas_instance_table_miss)
                pass

        # |action_spec| may be None.
        return match_spec, action_spec, entry_key

    def has_table_entry(self, entry_key):
        return entry_key in self._table_entries

    def get_table_entry(self, entry_key):
        if not self.has_table_entry(entry_key):
            return None
        return self._table_entries[entry_key]

    def add_table_entry(self, entry_key, entry_handler):
        if entry_key not in self._table_entries:
            self._table_entries[entry_key] = entry_handler

    def del_table_entry(self, entry_key):
        if entry_key in self._table_entries:
            del self._table_entries[entry_key]


# The SwitchController class.
class SwitchControlService(switch_rpc.SwitchControlServicer):
    _interface = None
    _managed_tables = set()
    _tables = {}
    _device_ports = {}
    _faas_channel = grpc.insecure_channel(kFaaSServerAddress)
    _flows = set()
    _packets_counter = 0
    _flows_counter = 0
    _sniffer_thread = None
    _stop = threading.Event()

    def __init__(self):
        # Catches the SIGINT (Ctrl+C) and SIGTSTP (Ctrl+Z).
        signal.signal(signal.SIGTSTP, self.sigsusp_handler)
        signal.signal(signal.SIGINT, self.sigint_handler)

    def sigint_handler(self, signum, frame):
        # Always clean up the system before killing the program.
        self.cleanup_system()
        sys.exit(0)

    def sigsusp_handler(self, signum, frame):
        print "Dump all table entries.."
        for table_name in self._tables:
            print "Table[%s]: %d entries" %(table_name, self.dump_table_entry(table_name))
        return

    def init_system(self):
        # Sets up the thrift connection with the switch ASIC.
        self._interface = ThriftInterface()

        self._managed_tables.add('faas_port_table')
        self._managed_tables.add('faas_conn_table')
        for table_name in self._managed_tables:
            self._tables[table_name] = SwitchTable(table_name)

        # |faas_port_table| has two actions.
        self._tables['faas_port_table'].add_action('faas_port_table_hit')
        self._tables['faas_port_table'].add_action('faas_port_table_miss')

        # |faas_conn_table| has two actions.
        self._tables['faas_conn_table'].add_action('faas_conn_table_hit')
        self._tables['faas_conn_table'].add_action('faas_conn_table_miss')

        # 'x/0' : (device port #, port sync mode)
        # 1/0, 19/0: ubuntu;
        self._device_ports['19/0'] = (kSwitchPortWorker1, 1)
        self._device_ports['1/0'] = (kSwitchPortWorker2, 1)
        # 21/0: uscnsl
        self._device_ports['21/0'] = (kSwitchPortTraffic, 2)

        self.setup_all_ports()

        self._flows = set()
        self._packets_counter = 0
        self._flows_counter = 0

    def cleanup_system(self):
        # Stops the packet sniffer.
        self._stop.set()
        if self._sniffer_thread != None:
            self._sniffer_thread.join()
        # Cleans up.
        self.delete_all_ports()
        self.delete_all_table_entries()
        self._interface.cleanup()
        self._interface = None
        return

    # This function notifies the switch ASIC to set up all related switch ports.
    def setup_all_ports(self):
        # Adds device ports.
        for port in self._device_ports.keys():
            dev_port, sync_mode = self._device_ports[port]
            self._interface.setup_dev_port(port, dev_port, sync_mode)

        # Adds forwarding rules at the faas_port_table via the thrift switch API.
        # Makes sure that packets from workers' ports go back to the traffic port.
        # Otherwise, flows are forwarded back to the original worker's port, and 
        # dropped by the NIC. This is unnecessary.
        table_name = "faas_port_table"
        action = "faas_port_table_hit"

        args = (kSwitchPortWorker1, kSwitchPortTraffic)
        match_spec, action_spec, entry_key = self._tables[table_name].get_match_action_spec(action, args)
        self.insert_table_entry(table_name, action, match_spec, action_spec, entry_key)

        args = (kSwitchPortWorker2, kSwitchPortTraffic)
        match_spec, action_spec, entry_key = self._tables[table_name].get_match_action_spec(action, args)
        self.insert_table_entry(table_name, action, match_spec, action_spec, entry_key)
        return

    # This function notifies the switch ASIC to disable all switch ports.
    def delete_all_ports(self):
        for port in self._device_ports.keys():
            self._interface.delete_dev_port(port)

    def dump_table_entry(self, table_name):
        if table_name not in self._tables:
            print "Error: invalid table name"
            return 0

        return self._interface.dump_table(table_name)

    def delete_table_entry(self, table_name, entry_key):
        if table_name not in self._tables:
            print "Error: invalid table name"
            return

        entry_handler = self._tables[table_name].get_table_entry(entry_key)
        if not entry_handler:
            # |entry_handler| is invalid.
            print "Error: no matching table entry"
            return

        res = self._interface.delete_exact_match_rule(table_name, entry_handler)
        if res != None:
            print "Error: failed to delete rule[%d]" %(entry_handler)
            return
        self._tables[table_name].del_table_entry(entry_key)

    # This function removes all existing table entries in the switch ASIC.
    def delete_all_table_entries(self):
        for table_name, table in self._tables.items():
            for entry_key in table._table_entries.keys():
                self.delete_table_entry(table._name, entry_key)

            assert(len(table._table_entries) == 0)
        return

    def insert_table_entry(self, table_name, action, match_spec, action_spec, entry_key):
        if table_name not in self._tables:
            print "Error: invalid table name"
            return
        if not self._tables[table_name].has_action(action):
            print "Error: invalid action"
            return
        if self._tables[table_name].has_table_entry(entry_key):
            #print "Error: Duplicated entry (flow)"
            return
        if self._interface == None:
            return

        #print self.dump_table_entry(table_name)
        # |entry_handler| is an integer that represents the rule index.
        entry_handler = self._interface.insert_exact_match_rule(table_name, action, match_spec, action_spec)
        # Stores the entry in a dict maintained by the table.
        self._tables[table_name].add_table_entry(entry_key, entry_handler)

    def packet_sniffer(self, interface):
        def _sniff(interface):
            sniff(iface=interface, \
                prn=lambda x: self.process_cpu_pkt(x), \
                stop_filter=lambda x: self.is_stop())

        self._sniffer_thread = threading.Thread(target=_sniff, args=(interface,))
        self._sniffer_thread.start()

    def process_cpu_pkt(self, packet):
        try:
            #print 'Get a packet'
            if IP not in packet or TCP not in packet:
                return
            self._packets_counter += 1

            table_name = "faas_conn_table"
            flowlet = (packet[IP].src, packet[IP].dst, packet[IP].proto, \
                packet[TCP].sport, packet[TCP].dport)
            if flowlet in self._flows:
                return

            # Inserts a placeholder for this new flow.
            self._flows.add(flowlet)
            self._flows_counter += 1

            flow_info = message_pb.FlowInfo()
            # Parses |flow_info| from the packet as it is the first packet of the flow.
            # e.g. ip_src = '204.57.7.6', ip_protocol = 6 (0x6), tcp_sport = 22
            flow_info.ipv4_src = packet[IP].src
            flow_info.ipv4_dst = packet[IP].dst
            flow_info.ipv4_protocol = packet[IP].proto
            flow_info.tcp_sport = packet[TCP].sport
            flow_info.tcp_dport = packet[TCP].dport

            faas_client = faas_rpc.FaaSControlStub(self._faas_channel)
            response = faas_client.UpdateFlow(flow_info)

            if response.dmac == "none":
                return

            binary_dmac = (response.dmac).replace(":", "").decode("hex")
            action = "faas_conn_table_hit"
            args = [packet[IP].src, packet[IP].dst, \
                packet[IP].proto, \
                packet[TCP].sport, packet[TCP].dport, \
                response.switch_port, binary_dmac]
            match_spec, action_spec, entry_key = self._tables[table_name].get_match_action_spec(action, args)

            # Calls the switch thrift API to insert the rule.
            self.insert_table_entry(table_name, action, match_spec, action_spec, entry_key)
        except Exception as e:
            print('Error:', e)

    def is_stop(self):
        return self._stop.is_set()

    ## The following functions implement gRPC server function calls.
    # |request| is an FlowTableEntry.
    def DeleteFlowEntry(self, request, context):
        table_name = "faas_conn_table"
        entry_key = (request.ipv4_src, request.ipv4_dst, request.ipv4_protocol, request.tcp_sport, request.tcp_dport)
        self.delete_table_entry(table_name, entry_key)
        return Empty()


class FaaSSwitchCLI(cmd.Cmd):
    _switch_controller = SwitchControlService()
    _grpc_server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    switch_rpc.add_SwitchControlServicer_to_server(_switch_controller, _grpc_server)
    _grpc_server.add_insecure_port(kSwitchServerAddress)
    # Two background threads.
    _grpc_thread = threading.Thread(target=_grpc_server.start()) # Set up gRPC server

    def preloop(self):
        self._switch_controller.init_system()
        return

    def postloop(self):
        self._switch_controller.cleanup_system()
        sys.exit(0)
        return

    def do_start(self, args):
        self._grpc_thread.start()

    # Receives packets from the switch ASIC-CPU channel.
    def do_recv(self, args):
        args = args.split()
        interface = 'bf_pci0'
        if len(args) > 0 and len(args[0]) > 0:
            interface = args[0]

        # Set up packet sniffer
        self._switch_controller.packet_sniffer(interface)
        return

    def do_stat(self, args):
        print "Traffic Info:"
        print "  Flows=%d\n" %(self._switch_controller._flows_counter)
        print "  Packets=%d\n" %(self._switch_controller._packets_counter)
        return

    def do_dump(self, args):
        args = args.split()

        if len(args) != 1:
            print "Error: incorrect table dump command"
            return

        table_name = args[0]
        print "Table[%s]: %d" %(table_name, self._switch_controller.dump_table_entry(table_name))
        return

    # insert faas_conn_table faas_conn_table_hit 0.0.0.1 0.0.0.2 6 1 2 192 00:11:22:33:44:55
    def do_insert(self, args):
        args = args.split()

        if len(args) <= 2:
            print "Error: incorrect rule insertion command"
            return

        table_name, action = args[0], args[1]
        match_action_spec = self._switch_controller._tables[table_name].get_match_action_spec(action, args[2:])
        if len(match_action_spec) == 3:
            match_spec = match_action_spec[0]
            action_spec = match_action_spec[1]
            entry_key = match_action_spec[2]
            #print "EntryKey=", entry_key
            self._switch_controller.insert_table_entry(table_name, action, match_spec, action_spec, entry_key)
        return

    def do_delete(self, args):
        args = args.split()

        if len(args) < 1:
            print "Error: incorrect rule deleting command"
            return

        table_name = args[0]
        entry_key = None
        if table_name == 'faas_conn_table':
            if len(args[1:]) < 5:
                return
            entry_key = (args[1], args[2], int(args[3], 0), int(args[4], 0), int(args[5], 0))
        elif table_name == "faas_instance_table":
            if len(args[1:]) < 2:
                return
            entry_key = (int(args[1], 0), int(args[2], 0))

        if entry_key:
            self._switch_controller.delete_table_entry(table_name, entry_key)
        return

    def do_help(self, args):
        print 'Available commands:'
        print '1. dump |tablename|: get the number entries in the table'
        print '2. receive |count|: receive |count| of packets from the switch ASIC'
        print '3. insert |tablename| |action| |match| |priority|: insert a rule into the switch'
        print '4. Exit/Quit'
        return

    def do_quit(self, args):
        return True


if __name__ == '__main__':
    controller = FaaSSwitchCLI()
    controller.prompt = '(faas-p4) '
    controller.cmdloop()
