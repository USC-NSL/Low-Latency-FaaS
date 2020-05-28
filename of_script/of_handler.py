
# This is the OpenFlow switch controller that works for the OpenFlow
# switch in a FaaS-NFV system.
# The controller sets up forwarding rules for each new flow.
# (1) Upon receiving the first packet of a flow, the switch controller
# queries FaaS-Controller to select all potentially related container
# instances, and encodes results in the NSH header.
# (2) Upon receiving all subsequent packets of a flow, the switch ensures
# that these packets go through the same path.

from ryu.base import app_manager
from ryu.controller import dp_event
from ryu.controller.handler import CONFIG_DISPATCHER, MAIN_DISPATCHER
from ryu.controller.handler import set_ev_cls
from ryu.ofproto import ofproto_v1_3
from ryu.lib.packet import packet
from ryu.lib.packet import 

import ofdpa.mods as Mods
import ofdpa.flow_description as FlowDescriptionReader

ryu_loggers = logging.Logger.manager.loggerDict
def ryu_logger_on(is_logger_on):
    for key in ryu_loggers.keys():
        ryu_logger = logging.getLogger(key)
        ryu_logger.propagate = is_logger_on

DLOG = logging.getLogger('ofdpa')
DLOG.setLevel(logging.DEBUG)


class FaaSSwitch(app_manager.RyuApp):
    _CONTEXTS = {'dp_event': dp_event.DPSet}
    OFP_VERSIONS = [ofproto_v1_3.OFP_VERSION]

    def __init__(self, *args, **kwargs):
        super(ExampleSwitch13, self).__init__(*args, **kwargs)

        # initialize mac address table.
        self.mac_to_port = {}

    # Install OpenFlow rules when the controller connects to the OpenFlow switch.
    @set_ev_cls(dp_event.EventDP, dp_event.DPSET_EV_DISPATCHER)
    def hanlder_datapath(self, ev):
        DLOG.info("Datapath ID: %i - 0x%x" %(ev.dp.id, ev.dp.id))
        if ev.enter:
            pass

    @set_ev_cls(ofp_event.EventOFPSwitchFeatures, CONFIG_DISPATCHER)
    def switch_features_handler(self, ev):
        datapath = ev.msg.datapath
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # Installs the table-miss flow entry.
        match = parser.OFPMatch()
        actions = [parser.OFPActionOutput(ofproto.OFPP_CONTROLLER,
                                          ofproto.OFPCML_NO_BUFFER)]
        self.add_flow(datapath, 0, match, actions)

    def add_flow(self, datapath, priority, match, actions):
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # construct flow_mod message and send it.
        inst = [parser.OFPInstructionActions(ofproto.OFPIT_APPLY_ACTIONS,
                                             actions)]
        mod = parser.OFPFlowMod(datapath=datapath, priority=priority,
                                match=match, instructions=inst)
        datapath.send_msg(mod)

    @set_ev_cls(ofp_event.EventOFPPacketIn, MAIN_DISPATCHER)
    def _packet_in_handler(self, ev):
        msg = ev.msg
        datapath = msg.datapath
        ofproto = datapath.ofproto
        parser = datapath.ofproto_parser

        # get Datapath ID to identify OpenFlow switches.
        dpid = datapath.id
        self.mac_to_port.setdefault(dpid, {})

        # analyse the received packets using the packet library.
        pkt = packet.Packet(msg.data)
        eth_pkt = pkt.get_protocol(ethernet.ethernet)
        dst = eth_pkt.dst
        src = eth_pkt.src

        # get the received port number from packet_in message.
        in_port = msg.match['in_port']

        self.logger.info("packet in %s %s %s %s", dpid, src, dst, in_port)

        # learn a mac address to avoid FLOOD next time.
        self.mac_to_port[dpid][src] = in_port

        # if the destination mac address is already learned,
        # decide which port to output the packet, otherwise FLOOD.
        if dst in self.mac_to_port[dpid]:
            out_port = self.mac_to_port[dpid][dst]
        else:
            out_port = ofproto.OFPP_FLOOD

        # construct action list.
        actions = [parser.OFPActionOutput(out_port)]

        # install a flow to avoid packet_in next time.
        if out_port != ofproto.OFPP_FLOOD:
            match = parser.OFPMatch(in_port=in_port, eth_dst=dst)
            self.add_flow(datapath, 1, match, actions)

        # construct packet_out message and send it.
        out = parser.OFPPacketOut(datapath=datapath,
                                  buffer_id=ofproto.OFP_NO_BUFFER,
                                  in_port=in_port, actions=actions,
                                  data=msg.data)
        datapath.send_msg(out)

    def build_openflow_packets(self, dp):
        rule_file = FlowDescriptionReader.get_config(self.CONFIG_FILE)

        for rule_idx, rule_config in enumerate(rule_file):
            # Each |rule_config| represents an OpenFlow rule, and must be a dict.
            assert(type(rule_config) == type({}))

            for config_type in FlowDescriptionReader.get_config_type(rule_config):
                if config_type == 'flow_mod':
                    # Handles the flow mode rules.
                    mod_config = FlowDescriptionReader.get_flow_mod(rule_config)
                    mod = Mods.create_flow_mod(dp, mod_config)
                    DLOG.info("Rule index=%d, Table=%s" %(rule_idx, rule_config['flow_mod']['table']))
                elif config_type == 'group_mod':
                    # Handles the group mode rules.
                    mod_config = FlowDescriptionReader.get_group_mod(rule_config)
                    mod = Mods.create_group_mod(dp, mod_config)
                else:
                    raise Exception("Error: OpenFlow rule mode")

            dp.send_msg(mod)
