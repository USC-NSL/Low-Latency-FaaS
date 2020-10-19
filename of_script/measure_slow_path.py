
# This OpenFlow script measures the flow-installation rate.

import time
import os
import sys
import json
import struct
import copy
import time
import collections
import gflags
import logging
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

# Protobuf and GRPC.
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

kDefaultMonitoringPeriod = 10
kDefaultIdleTimeout = 10

ryu_loggers = logging.Logger.manager.loggerDict
def ryu_logger_on(is_logger_on):
    for key in ryu_loggers.keys():
        ryu_logger = logging.getLogger(key)
        ryu_logger.propagate = is_logger_on

DLOG = logging.getLogger('ofdpa')
DLOG.setLevel(logging.DEBUG)


class FaaSSwitchController(app_manager.RyuApp):
    OFP_VERSIONS = [
        ofproto_v1_0.OFP_VERSION,
        ofproto_v1_2.OFP_VERSION,
        ofproto_v1_3.OFP_VERSION
    ]
    _CONTEXTS = {
        'dpset': dpset.DPSet,
    }

    def __init__(self, *args, **kwargs):
        super(FaaSSwitchController, self).__init__(*args, **kwargs)

        self.monitor_thread = hub.spawn(self._monitor)

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
        self._global_pkts_counter = 0
        self._last_lost_pkts_ts = time.time()
        self._last_global_pkts_counter = 0

        # internal flow table
        self._flows = set()
        self._flows_pkt_count = {}
        self._flows_table_entries = {}

    # The background monitoring function.
    # Outputs the message every 10-second.
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

            self._print_pkt_in_stats()

            hub.sleep(kDefaultMonitoringPeriod)

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

    def _print_pkt_in_stats(self):
        now = time.time()
        lost_rate = (self._global_pkts_counter - self._last_global_pkts_counter) / (now - self._last_lost_pkts_ts)
        self._last_global_pkts_counter = self._global_pkts_counter
        self._last_lost_pkts_ts = now
        DLOG.info("Incoming packet rate: %d pkts/s", lost_rate)

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
        self.add_flow(datapath, match_0, actions_0, 100, priority=0, idle_timeout=0)

        match_1 = parser.OFPMatch()
        actions_1 = [parser.OFPActionOutput(ofproto.OFPP_CONTROLLER, ofproto.OFPCML_NO_BUFFER)]
        self.add_flow(datapath, match_1, actions_1, 200, priority=0, idle_timeout=0)

        ipv4_src, ipv4_dst = "10.0.0.1", "10.0.0.2"
        start_time = time.time()
        for i in range(1000):
            actions = [parser.OFPActionSetField(eth_dst="00:00:00:00:00:01")]
            # install a flow to avoid packet_in next time.
            match = parser.OFPMatch(eth_type=ether_types.ETH_TYPE_IP, 
                ipv4_src=ipv4_src, ipv4_dst=ipv4_dst, ip_proto=in_proto.IPPROTO_TCP, 
                tcp_src=9000+i, tcp_dst=8080)
            self.add_flow(datapath, match, actions, 200, priority=3, idle_timeout=0)

        while datapath.send_q.qsize() != 0:
            print datapath.send_q.qsize()
            time.sleep(1)
        end_time = time.time()

        print "rate=%d" %(1000 / (end_time - start_time))

    def add_flow(self, datapath, match, actions, table_id, priority, idle_timeout):
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

        # This FlowMod matches all flows. So it will delete all flows at table 200.
        match = parser.OFPMatch()
        actions = []
        self.delete_flow(datapath, 0, match, actions, 200)
        self.delete_flow(datapath, 1, match, actions, 200)
        DLOG.info("Delete all existing flows..")

    @set_ev_cls(ofp_event.EventOFPPacketIn, MAIN_DISPATCHER)
    def _packet_in_handler(self, ev):
        msg = ev.msg
        datapath = msg.datapath
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # analyse the received packets using the packet library.
        pkt = packet.Packet(msg.data)
        eth_pkt = pkt.get_protocol(ethernet.ethernet)
        eth_src = eth_pkt.src
        eth_dst = eth_pkt.dst

        self._global_pkts_counter += 1
        return


if __name__ == "__main__":
    pass
