# This is the OpenFlow switch controller that works for the OpenFlow
# switch in a FaaS-NFV system.
# The controller sets up forwarding rules for each new flow.

import os
import sys
import time
import json
import struct
import copy
import collections
import logging
import gflags
from operator import attrgetter
from ryu.base import app_manager
from ryu.app import simple_switch_13
from ryu.controller import dpset, ofp_event
from ryu.controller.handler import MAIN_DISPATCHER, CONFIG_DISPATCHER, DEAD_DISPATCHER
from ryu.controller.handler import set_ev_cls
from ryu.ofproto import ofproto_v1_0
from ryu.ofproto import ofproto_v1_2
from ryu.ofproto import ofproto_v1_3
from ryu.lib import hub
from ryu.lib.packet import packet, ethernet, ether_types, ipv4, in_proto, icmp, tcp, udp
from ryu.lib import ofctl_v1_0
from ryu.lib import ofctl_v1_2
from ryu.lib import ofctl_v1_3

# No ofdpa usage.
#import ofdpa.mods as Mods
#import ofdpa.flow_description as FlowDescriptionReader

# Protobuf and GRPC.
import redis
import grpc
from concurrent import futures
from google.protobuf.empty_pb2 import Empty
import protobuf.message_pb2 as message_pb
import protobuf.switch_service_pb2 as switch_pb
import protobuf.switch_service_pb2_grpc as switch_rpc
import protobuf.faas_service_pb2 as faas_pb
import protobuf.faas_service_pb2_grpc as faas_rpc


supported_ofctl = {
    ofproto_v1_0.OFP_VERSION: ofctl_v1_0,
    ofproto_v1_2.OFP_VERSION: ofctl_v1_2,
    ofproto_v1_3.OFP_VERSION: ofctl_v1_3,
}

FLAGS = gflags.FLAGS
gflags.DEFINE_string("faas_ip", "128.105.145.66", "FaaS Controller's IP")
gflags.DEFINE_string("faas_port", "10515", "FaaS Controller service's Port")
gflags.DEFINE_boolean("verbose", False, "Verbose logs")
gflags.DEFINE_boolean("pkt", True, "Monitor the number of packets from each flow")
gflags.DEFINE_integer("inport", 44, "The switch port as the traffic ingress")

kFaaSServerAddress = "%s:%s" %(FLAGS.faas_ip, FLAGS.faas_port)
kSwitchServiceOn = False
kSwitchServerAddress = "[::]:10516"
kDefaultMonitoringPeriod = 20
kDefaultIdleTimeout = 60


ryu_loggers = logging.Logger.manager.loggerDict
def ryu_logger_on(is_logger_on):
    for key in ryu_loggers.keys():
        ryu_logger = logging.getLogger(key)
        ryu_logger.propagate = is_logger_on

DLOG = logging.getLogger('ofdpa')
DLOG.setLevel(logging.DEBUG)


def stats_method(method):
    def wrapper(self, req, dpid, *args, **kwargs):
        # Get datapath instance from DPSet
        try:
            dp = self.dpset.get(dpid)
        except ValueError:
            LOG.exception('Invalid dpid: %s', dpid)
            return Response(status=400)
        if dp is None:
            LOG.error('No such Datapath: %s', dpid)
            return Response(status=404)

        # Get lib/ofctl_* module
        try:
            ofctl = supported_ofctl.get(dp.ofproto.OFP_VERSION)
        except KeyError:
            LOG.exception('Unsupported OF version: %s',
                          dp.ofproto.OFP_VERSION)
            return Response(status=501)

        # Invoke StatsController method
        try:
            ret = method(self, req, dp, ofctl, *args, **kwargs)
            return Response(content_type='application/json',
                            body=json.dumps(ret))
        except ValueError:
            LOG.exception('Invalid syntax: %s', req.body)
            return Response(status=400)
        except AttributeError:
            LOG.exception('Unsupported OF request in this version: %s',
                          dp.ofproto.OFP_VERSION)
            return Response(status=501)

    return wrapper


class SwitchControlService(switch_rpc.SwitchControlServicer):
    """ The SwitchController gRPC server. (Not used)
    This class is the main gRPC server that implements operations related
    to a OpenFlow switch.
    Args:
        _of_interface: a FaaSRuleInstaller interacting with the switch
    """
    _of_interface = None

    def __init__(self, of_interface):
        self._of_interface = of_interface

    ## The following functions implement gRPC server function calls.
    # |request| is an FlowTableEntry.
    def InsertFlowEntry(self, request, context):
        print "good"
        if self._of_interface:
            self._of_interface.process_new_flow(request)
        return Empty()

    def DeleteFlowEntry(self, request, context):
        return Empty()


class FaaSRuleInstaller(app_manager.RyuApp):
    """ The rule installer class.
    This class interacts with the switch hardware to install OpenFlow rules,
    and provide monitoring functions.
    """
    OFP_VERSIONS = [
        ofproto_v1_0.OFP_VERSION,
        ofproto_v1_2.OFP_VERSION,
        ofproto_v1_3.OFP_VERSION
    ]
    _CONTEXTS = {
        'dpset': dpset.DPSet,
    }

    _workers = []
    _grpc_server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    _switch_controller = None

    def __init__(self, *args, **kwargs):
        super(FaaSRuleInstaller, self).__init__(*args, **kwargs)

        # Ryu uses eventlet, which is imcompatible with the native threads.
        self._switch_controller = SwitchControlService(self)
        switch_rpc.add_SwitchControlServicer_to_server(self._switch_controller, self._grpc_server)
        self._grpc_server.add_insecure_port(kSwitchServerAddress)

        self.dpset = kwargs['dpset']
        #wsgi = kwargs['wsgi']

        self.waiters = {}
        self.data = {}
        self.data['dpset'] = self.dpset
        self.data['waiters'] = self.waiters

        # initialize all data paths,
        self.datapaths = {}

        # initialize mac_to_port tables
        self.mac_to_port = {}

        # internal counters
        self._flows_counter = 0
        self._pkts_counter = 0
        self._lost_pkts_counter = 0
        self._last_lost_pkts_ts = time.time()
        self._last_lost_pkts_counter = 0

        # internal flow table
        self._flows = set()
        self._flows_pkt_count = {}
        self._flows_table_entries = {}

        self.monitor_thread = hub.spawn(self._monitor)

        if not kSwitchServiceOn:
            self._redis_conn = redis.Redis(host='127.0.0.1', port=6379, password='faas-nfv-cool')
            self._redis_sub = self._redis_conn.pubsub()
            self._redis_sub.subscribe('flow')
            self.flow_thread = hub.spawn(self._handle_flows)

    # The background monitoring function.
    # Outputs the message every 10-second.
    def _monitor(self):
        while True:
            for dp in self.datapaths.values():
                ofproto = dp.ofproto
                parser = dp.ofproto_parser
                flow_request_str = "10.0.0.28,10.0.0.12,6,10001,8080,21,00:00:00:00:00:02"
                flow_request = flow_request_str.split(',')
                select_port = int(flow_request[5])
                select_dmac = flow_request[6]
                actions = [parser.OFPActionSetField(eth_dst=select_dmac),
                        parser.OFPActionOutput(select_port)]
                match = parser.OFPMatch(eth_type=ether_types.ETH_TYPE_IP,
                        ipv4_src=flow_request[0], ipv4_dst=flow_request[1],
                        ip_proto=int(flow_request[2]),
                        tcp_src=int(flow_request[3]), tcp_dst=int(flow_request[4]))
                #self._add_flow(dp, match, actions, 200, priority=2, idle_timeout=kDefaultIdleTimeout)

                try:
                    ofctl = supported_ofctl.get(dp.ofproto.OFP_VERSION)
                except KeyError:
                    DLOG.error("Invalid OFP version")
                    ofctl = None

                if ofctl:
                    try:
                        flow_ret = ofctl.get_flow_stats(dp, self.waiters, None)
                        port_ret = ofctl.get_port_stats(dp, self.waiters, None)
                    except:
                        DLOG.error("Failed to dump port info")

                    self._print_formatted_flow_stats(flow_ret)
                    self._print_formatted_port_stats(port_ret)

            if FLAGS.pkt:
                self._print_pkt_stats()
                self._print_loss_stats()

            hub.sleep(kDefaultMonitoringPeriod)

    def _handle_flows(self):
        while True:
            message = self._redis_sub.get_message()
            if message and message['data']:
                if isinstance(message['data'], basestring):
                    self.process_new_flow(message['data'])

            hub.sleep(0.002)

    def _request_stats(self, datapath):
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        req = parser.OFPFlowStatsRequest(datapath)
        datapath.send_msg(req)

        req = parser.OFPPortStatsRequest(datapath, 0, ofproto.OFPP_ANY)
        datapath.send_msg(req)

    def _print_formatted_flow_stats(self, flow_stats):
        if len(flow_stats) != 1:
            return

        DLOG.info('match         action       '
                  'packets  bytes')
        DLOG.info('---------------- '
                  '-------- ----------------- '
                  '-------- -------- --------')

        datapath = list(flow_stats.keys())[0]
        body = flow_stats[datapath]

        if FLAGS.verbose:
            for flow in body:
                DLOG.info('match:%17s actions:%17s pkts:%8d bytes:%8d',
                          str(flow["match"]), str(flow["actions"]), 
                          flow["packet_count"], flow["byte_count"])
        DLOG.info("Total %d flows", len(body))

    def _print_formatted_port_stats(self, port_stats):
        DLOG.info('datapath           port_no '
                  'rx-pkts  rx-bytes rx-error '
                  'tx-pkts  tx-bytes tx-error')
        DLOG.info('---------------- -------- '
                  '-------- -------- -------- '
                  '-------- -------- --------')

        datapath = list(port_stats.keys())[0]
        body = port_stats[datapath]
        for stat in sorted(body, key=lambda s: s["port_no"]):
            DLOG.info('%16s %8x %8d %8d %8d %8d %8d %8d',
                             datapath, stat["port_no"],
                             stat["rx_packets"], stat["rx_bytes"], stat["rx_errors"], 
                             stat["tx_packets"], stat["tx_bytes"], stat["tx_errors"])

    def _print_pkt_stats(self):
        pkt_samples = sorted(self._flows_pkt_count.values())
        count_flows = len(pkt_samples)
        if count_flows == 0:
            DLOG.info("No flow yet")
            return

        avg_val = sum(pkt_samples) / count_flows
        min_val, max_val = pkt_samples[0], pkt_samples[-1]
        p50_val, p95_val = pkt_samples[int(0.5*count_flows)], pkt_samples[int(0.95*count_flows)]

        DLOG.info("Subsequent pkts from a flow:")
        DLOG.info("total flows: %d, avg: %d, min: %d, 50pct: %d, 95pct: %d, max: %d", \
            count_flows, avg_val, min_val, p50_val, p95_val, max_val)

    def _print_loss_stats(self):
        now = time.time()
        lost_rate = (self._lost_pkts_counter - self._last_lost_pkts_counter) / (now - self._last_lost_pkts_ts)
        self._last_lost_pkts_counter = self._lost_pkts_counter
        self._last_lost_pkts_ts = now
        DLOG.info("Lost packet rate: %d pps", lost_rate)

    def _update_per_flow_pkt_stats(self, flowlet):
        if flowlet not in self._flows_pkt_count:
            self._flows_pkt_count[flowlet] = 1
        else:
            self._flows_pkt_count[flowlet] += 1

    # Deletes the stored msg in |self.msgs| after we are done with it.
    # Note: each msg is tagged by its transaction ID, i.e. |msg.xid|.
    @set_ev_cls([ofp_event.EventOFPStatsReply,
                 ofp_event.EventOFPDescStatsReply,
                 ofp_event.EventOFPFlowStatsReply,
                 ofp_event.EventOFPAggregateStatsReply,
                 ofp_event.EventOFPTableStatsReply,
                 ofp_event.EventOFPTableFeaturesStatsReply,
                 ofp_event.EventOFPPortStatsReply,
                 ofp_event.EventOFPQueueStatsReply,
                 ofp_event.EventOFPQueueDescStatsReply,
                 ofp_event.EventOFPMeterStatsReply,
                 ofp_event.EventOFPMeterFeaturesStatsReply,
                 ofp_event.EventOFPMeterConfigStatsReply,
                 ofp_event.EventOFPGroupStatsReply,
                 ofp_event.EventOFPGroupFeaturesStatsReply,
                 ofp_event.EventOFPGroupDescStatsReply,
                 ofp_event.EventOFPPortDescStatsReply
                 ], MAIN_DISPATCHER)
    def stats_reply_handler(self, ev):
        msg = ev.msg
        dp = msg.datapath

        if dp.id not in self.waiters:
            return
        if msg.xid not in self.waiters[dp.id]:
            return
        lock, msgs = self.waiters[dp.id][msg.xid]
        msgs.append(msg)

        flags = 0
        if dp.ofproto.OFP_VERSION == ofproto_v1_0.OFP_VERSION:
            flags = dp.ofproto.OFPSF_REPLY_MORE
        elif dp.ofproto.OFP_VERSION == ofproto_v1_2.OFP_VERSION:
            flags = dp.ofproto.OFPSF_REPLY_MORE
        elif dp.ofproto.OFP_VERSION >= ofproto_v1_3.OFP_VERSION:
            flags = dp.ofproto.OFPMPF_REPLY_MORE

        if msg.flags & flags:
            return
        del self.waiters[dp.id][msg.xid]
        lock.set()

    # Called when a new switch (DP) joins the controller.
    @set_ev_cls(ofp_event.EventOFPStateChange, [MAIN_DISPATCHER, DEAD_DISPATCHER])
    def _state_change_handler(self, ev):
        datapath = ev.datapath
        if ev.state == MAIN_DISPATCHER:
            if datapath.id not in self.datapaths:
                DLOG.info('register datapath: %016x, %s', datapath.id, str(datapath.id))
                self.datapaths[datapath.id] = datapath
        elif ev.state == DEAD_DISPATCHER:
            if datapath.id in self.datapaths:
                DLOG.info('unregister datapath: %016x, %s', datapath.id, str(datapath.id))
                del self.datapaths[datapath.id]

    # Called when the switch is ready and waiting for initial configurations.
    @set_ev_cls(ofp_event.EventOFPSwitchFeatures, CONFIG_DISPATCHER)
    def switch_features_handler(self, ev):
        DLOG.info("Enter config dispatcher")
        datapath = ev.msg.datapath
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # Deletes existing flows.
        self.delete_all_flows(datapath)

        # The MAC-IP table (table ID: 100)
        # The default rule that forwards all packets to the next table.
        match_0 = parser.OFPMatch()
        actions_0 = []
        self._add_flow(datapath, match_0, actions_0, 100, priority=0, idle_timeout=0)

        # The extension table (table ID: 200)
        # Installs the table-miss flow entry for ingress unseen traffic.
        match_1 = parser.OFPMatch(in_port=FLAGS.inport)
        actions_1 = [parser.OFPActionOutput(ofproto.OFPP_CONTROLLER, ofproto.OFPCML_NO_BUFFER)]
        self._add_flow(datapath, match_1, actions_1, 200, priority=0, idle_timeout=0)

        # Installs the egress rules for egress traffic.
        match_2 = parser.OFPMatch(eth_dst="00:15:4d:12:2b:f4", eth_type=ether_types.ETH_TYPE_IP)
        actions_2 = [parser.OFPActionOutput(FLAGS.inport)]
        self._add_flow(datapath, match_2, actions_2, 200, priority=3, idle_timeout=0)

        if kSwitchServiceOn:
            self._grpc_server.start()
        return

    def _add_flow(self, datapath, match, actions, table_id, priority, idle_timeout):
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser
        hard_timeout = 0

        # construct the flow instruction
        inst = []
        if actions != None and len(actions) > 0:
            inst.append(parser.OFPInstructionActions(
                                ofproto.OFPIT_APPLY_ACTIONS, actions))
        if table_id == 100 and len(actions) == 0:
            inst.append(parser.OFPInstructionGotoTable(200))

        # construct flow_mod message and send it.
        mod = parser.OFPFlowMod(datapath=datapath, table_id=table_id,
                                command=ofproto.OFPFC_ADD,
                                idle_timeout=idle_timeout, hard_timeout=hard_timeout,
                                priority=priority, buffer_id= ofproto.OFP_NO_BUFFER,
                                out_port=ofproto.OFPP_ANY, out_group=ofproto.OFPG_ANY,
                                flags=ofproto.OFPFF_SEND_FLOW_REM,
                                match=match, instructions=inst)
        datapath.send_msg(mod)

    def _delete_flow(self, datapath, priority, match, actions, table_id):
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # construct the flow instruction
        inst = []
        if actions != None and len(actions) > 0:
            inst.append(parser.OFPInstructionActions(
                                ofproto.OFPIT_APPLY_ACTIONS, actions))

        # construct flow_mod message and send it.
        mod = parser.OFPFlowMod(datapath=datapath, priority=priority,
                                command=ofproto.OFPFC_DELETE, 
                                match=match, instructions=inst,
                                out_port=ofproto.OFPP_ANY, out_group=ofproto.OFPP_ANY, table_id=table_id)
        datapath.send_msg(mod)

    def delete_all_flows(self, datapath):
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # This FlowMod matches all flows. So it will delete all flows at table 200.
        match = parser.OFPMatch()
        actions = []
        self._delete_flow(datapath, 0, match, actions, 200)
        self._delete_flow(datapath, 1, match, actions, 200)
        self._delete_flow(datapath, 2, match, actions, 200)
        self._delete_flow(datapath, 3, match, actions, 200)
        DLOG.info("Delete all existing flows..")

    @set_ev_cls(ofp_event.EventOFPPacketIn, MAIN_DISPATCHER)
    def _packet_in_handler(self, ev):
        """ FaaSIngress handles incoming flows as a sevice. Ideally, 
        this Ryu controller should expect no FlowIn message. However,
        before the rule is installed, subsequent packets may arrive and
        be forwarded here. The controller considers them as losses.
        """
        # msg = ev.msg
        # datapath = msg.datapath
        # ofproto = datapath.ofproto
        # parser = datapath.ofproto_parser
        self._lost_pkts_counter += 1

    def process_new_flow(self, flow_request_str):
        """ Install a flow to avoid packet_in next time.
        Here is one example flow request:
            10.0.0.28,10.0.0.12,6,10001,8080,21,00:00:00:00:00:02
        Args:
            flow_request_str: the incoming flow request in the str format
        """
        datapath = self.datapaths.values()[0]
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser
        flow_request = flow_request_str.split(',')

        select_port = int(flow_request[5])
        select_dmac = flow_request[6]
        actions = [parser.OFPActionSetField(eth_dst=select_dmac),
                parser.OFPActionOutput(select_port)]
        match = parser.OFPMatch(eth_type=ether_types.ETH_TYPE_IP,
                ipv4_src=flow_request[0], ipv4_dst=flow_request[1],
                ip_proto=int(flow_request[2]),
                tcp_src=int(flow_request[3]), tcp_dst=int(flow_request[4]))
        self._add_flow(datapath, match, actions, 200, priority=2, idle_timeout=kDefaultIdleTimeout)


if __name__ == "__main__":
    pass
