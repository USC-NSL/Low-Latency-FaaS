
# This is the OpenFlow switch controller that works for the OpenFlow
# switch in a FaaS-NFV system.
# The controller sets up forwarding rules for each new flow.
# (1) Upon receiving the first packet of a flow, the switch controller
# queries FaaS-Controller to select all potentially related container
# instances, and encodes results in the NSH header.
# (2) Upon receiving all subsequent packets of a flow, the switch ensures
# that these packets go through the same path.

import os
import sys
import json
import struct
import logging
import time
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

#import ofdpa.mods as Mods
#import ofdpa.flow_description as FlowDescriptionReader


supported_ofctl = {
    ofproto_v1_0.OFP_VERSION: ofctl_v1_0,
    ofproto_v1_2.OFP_VERSION: ofctl_v1_2,
    ofproto_v1_3.OFP_VERSION: ofctl_v1_3,
}


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

# stats_method:
# dp = self.dpset.get(dpid)
# ofctl = supported_ofctl.get(dp.ofproto.OFP_VERSION)
# ofctl.get_port_stats(dp, self.waiters, port)
# ofproto.OFPP_ANY

"""
Port-stats example:
{"329655727540867208": [
{"rx_packets": 404359455600, "tx_packets": 335211930257, "rx_bytes": 48392579858726, "tx_bytes": 62095263878913, "rx_dropped": 18446744073709551615, "tx_dropped": 18446744073709551615, "rx_errors": 1, "tx_errors": 0, "rx_frame_err": 0, "rx_over_err": 18446744073709551615, "rx_crc_err": 1, "collisions": 0, "duration_sec": 5136, "duration_nsec": 4294967295, "port_no": 9}, 
{"rx_packets": 10966378733, "tx_packets": 20218063366, "rx_bytes": 6650538766267, "tx_bytes": 10516456661239, "rx_dropped": 18446744073709551615, "tx_dropped": 18446744073709551615, "rx_errors": 0, "tx_errors": 0, "rx_frame_err": 0, "rx_over_err": 18446744073709551615, "rx_crc_err": 0, "collisions": 0, "duration_sec": 5136, "duration_nsec": 4294967295, "port_no": 15}, 
{"rx_packets": 42037050617, "tx_packets": 39946272732, "rx_bytes": 23003964479582, "tx_bytes": 35601832555933, "rx_dropped": 18446744073709551615, "tx_dropped": 18446744073709551615, "rx_errors": 0, "tx_errors": 0, "rx_frame_err": 0, "rx_over_err": 18446744073709551615, "rx_crc_err": 0, "collisions": 0, "duration_sec": 5136, "duration_nsec": 4294967295, "port_no": 17}]}

Flow-stats example:
{"329655727540867208": [{"priority": 0, "cookie": 0, "idle_timeout": 0, "hard_timeout": 0, "byte_count": 18446744073709551615, "duration_sec": 61, "duration_nsec": 4294967295, "packet_count": 18446744073709551615, "length": 80, "flags": 0, "actions": ["OUTPUT:CONTROLLER"], "match": {}, "table_id": 100}]}
"""

class FaaSSwitch(app_manager.RyuApp):
    OFP_VERSIONS = [
        ofproto_v1_0.OFP_VERSION,
        ofproto_v1_2.OFP_VERSION,
        ofproto_v1_3.OFP_VERSION
    ]
    _CONTEXTS = {
        'dpset': dpset.DPSet,
        #'wsgi': WSGIApplication
    }

    def __init__(self, *args, **kwargs):
        super(FaaSSwitch, self).__init__(*args, **kwargs)

        self.monitor_thread = hub.spawn(self._monitor)

        self.dpset = kwargs['dpset']
        #wsgi = kwargs['wsgi']

        self.waiters = {}
        self.data = {}
        self.data['dpset'] = self.dpset
        self.data['waiters'] = self.waiters

        # initialize all data paths,
        self.datapaths = {}

        # initialize mac address table.
        self.mac_to_port = {}

    # The background monitoring function.
    # Outputs the monitoring message every 10-second.
    def _monitor(self):
        while True:
            for dp in self.datapaths.values():
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

            hub.sleep(10)

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
        for flow in body:
            DLOG.info('%17s %17s %8d %8d',
                      str(flow["match"]), str(flow["actions"]), 
                      flow["packet_count"], flow["byte_count"])

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

        # Adds default rules.
        match = parser.OFPMatch()
        actions = [parser.OFPActionOutput(ofproto.OFPP_CONTROLLER,
                                          ofproto.OFPCML_NO_BUFFER)]

        # Bypass the MAC-IP table.
        match_1 = parser.OFPMatch()
        actions_1 = []
        self.add_flow(datapath, 0, match_1, actions_1, 100)

        # Installs the table-miss flow entry.
        match_2 = parser.OFPMatch()
        actions_2 = [parser.OFPActionOutput(ofproto.OFPP_CONTROLLER,
                                          ofproto.OFPCML_NO_BUFFER)]
        self.add_flow(datapath, 0, match_2, actions_2, 200)

        """
        # Installs rules for testing.
        match_3 = parser.OFPMatch(eth_src="11:11:11:11:11:11", eth_type=ether_types.ETH_TYPE_IP, 
            ipv4_src="10.0.0.1", ipv4_dst="10.0.0.2", ip_proto=in_proto.IPPROTO_TCP, 
            tcp_src=1234, tcp_dst=4321)
        actions_3 = [parser.OFPActionSetField(eth_dst="00:00:00:00:00:01"),
            parser.OFPActionOutput(17)]
        self.add_flow(datapath, 1, match_3, actions_3, 200)

        match_4 = parser.OFPMatch(in_port=17)
        actions_4 = [parser.OFPActionOutput(9)]
        self.add_flow(datapath, 1, match_4, actions_4, 200)
        """

    def add_flow(self, datapath, priority, match, actions, table_id):
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # construct the flow instruction
        inst = []
        if actions != None and len(actions) > 0:
            inst.append(parser.OFPInstructionActions(
                                ofproto.OFPIT_APPLY_ACTIONS, actions))
        if table_id == 100:
            inst.append(parser.OFPInstructionGotoTable(200))

        # construct flow_mod message and send it.
        mod = parser.OFPFlowMod(datapath=datapath, priority=priority,
                                command=ofproto.OFPFC_ADD,
                                match=match, instructions=inst, table_id=table_id)
        datapath.send_msg(mod)

    def delete_flow(self, datapath, priority, match, actions, table_id):
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

        match = parser.OFPMatch()
        actions = []
        self.delete_flow(datapath, 1, match, inst, 200)

    @set_ev_cls(ofp_event.EventOFPPacketIn, MAIN_DISPATCHER)
    def _packet_in_handler(self, ev):
        DLOG.info("Receive a packet.")
        return

        msg = ev.msg
        datapath = msg.datapath
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # get Datapath ID to identify OpenFlow switches.
        dpid = datapath.id
        self.mac_to_port.setdefault(dpid, {})

        # get the received port number from packet_in message.
        in_port = msg.match['in_port']

        # analyse the received packets using the packet library.
        pkt = packet.Packet(msg.data)
        eth_pkt = pkt.get_protocol(ethernet.ethernet)
        eth_src = eth_pkt.src
        eth_dst = eth_pkt.dst

        if eth_dst in self.mac_to_port[dpid]:
            out_port = self.mac_to_port[dpid][eth_dst]
        else:
            out_port = ofproto.OFPP_FLOOD

        # construct action list.
        actions = [parser.OFPActionOutput(out_port)]


        # Helper functions on processing packets.
        def handle_icmp_pkt(icmp_pkt):
            if icmp_pkt == None:
                return

            DLOG.info("%d: ICMP pkt in port[%s]: (%s,%s)", dpid, in_port, eth_src, eth_dst)

            # install a flow to avoid packet_in next time.
            if out_port != ofproto.OFPP_FLOOD:
                match = parser.OFPMatch(eth_dst=eth_dst, eth_type=ether_types.ETH_TYPE_IP)
                self.add_flow(datapath, 1, match, actions, 200)

        def handle_ipv4_pkt(ipv4_pkt):
            if ipv4_pkt == None:
                return

            ipv4_src = ipv4_pkt.src
            ipv4_dst = ipv4_pkt.dst
            DLOG.info("%d: IP pkt in port[%s] (%s,%s,%s,%s)", dpid, in_port, eth_src, eth_dst, ipv4_src, ipv4_dst)

            select_port = "13"
            actions = [parser.OFPActionOutput(select_port)]
            # install a flow to avoid packet_in next time.
            match = parser.OFPMatch(eth_dst=eth_dst, eth_type=ether_types.ETH_TYPE_IP, 
                ipv4_src=ipv4_src, ipv4_dst=ipv4_dst)
            self.add_flow(datapath, 2, match, actions, 200)


        icmp_pkt = pkt.get_protocol(icmp.icmp)
        if icmp_pkt:
            handle_icmp_pkt(icmp_pkt)

        ipv4_pkt = pkt.get_protocol(ipv4.ipv4)
        if ipv4_pkt:
            handle_ipv4_pkt(ipv4_pkt)

        # learn a mac address to avoid FLOOD next time.
        self.mac_to_port[dpid][eth_src] = in_port

        # construct packet_out message and send it.
        out = parser.OFPPacketOut(datapath=datapath,
                                  buffer_id=ofproto.OFP_NO_BUFFER,
                                  in_port=in_port, actions=actions,
                                  data=msg.data)
        datapath.send_msg(out)


if __name__ == "__main__":
    pass
