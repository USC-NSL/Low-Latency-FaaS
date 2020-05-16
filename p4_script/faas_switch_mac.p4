
/* -*_ p4_14 -*- */
#include </root/jianfeng/tofino/intrinsic_metadata.p4>
#include </root/jianfeng/tofino/constants.p4>

#define CPU_PORT 192
#define TYPE_VLAN 0x0810
#define TYPE_NSH 0x894F
#define TYPE_NSH_IPV4 0x1
#define TYPE_NSH_VLAN 0x6
#define TYPE_IPV4 0x0800
#define TYPE_IPV6 0x0816
#define CONN_TABLE_SIZE 20480
#define PORT_TABLE_SIZE 64
#define INSTANCE_TABLE_SIZE 1000
#define TYPE_TCP 0x06
#define TYPE_UDP 0x11
#define IPV4_FORWARD_TABLE_SIZE 5000

/*******************************************************************
************************  H E A D E R ******************************
********************************************************************/

header_type ethernet_t {
	fields{
		dstAddr : 48;
		srcAddr : 48;
		etherType : 16;
	}
}

header_type vlan_t {
	fields{
		TCI : 16;
		nextType : 16;
	}
}

header_type nsh_t {
	fields{
		version : 2;
		oBit : 1;
		uBit : 1;
		ttl : 6;
		totalLength : 6;
		unsign : 4;
		md : 4;
		nextProto : 8;
		spi : 24;
		si : 8;
		context : 128;
	}
}

header_type ipv4_t {
	fields{
		version : 4;
		ihl : 4;
		diffserv : 8;
		totalLen : 16;
		identification : 16;
		flags : 3;
		fragOffset : 13;
		ttl : 8;
		protocol : 8;
		hdrChecksum : 16;
		srcAddr : 32;
		dstAddr : 32;
	}
}

header_type ipv6_t {
	fields{
		version : 4;
		traffic_class : 8;
		flow_label : 20;
		payload_len : 16;
		next_hdr : 8;
		hop_limit : 8;
		srcAddr : 64;
		dstAddr : 64;
	}
}

header_type tcp_t {
	fields{
		srcPort : 16;
		dstPort : 16;
		seqNo : 32;
		ackNo : 32;
		dataOffset : 4;
		res : 3;
		ecn : 3;
		ctrl : 6;
		window : 16;
		checksum : 16;
		urgentPtr : 16;
	}
}

header_type udp_t {
	fields{
		srcPort : 16;
		dstPort : 16;
		hdr_length : 16;
		checksum : 16;
	}
}


header_type metadata_t {
	fields {
		spi : 16;
		si : 16;
		controller_flag : 1;
		drop_flag : 1;
		port_table_miss_flag : 4;
		conn_table_miss_flag : 4;
		instance_table_miss_flag : 4;
	}
}

metadata metadata_t meta;

header ethernet_t ethernet;
header vlan_t vlan;
header nsh_t nsh;
header ipv4_t ipv4;
header ipv6_t ipv6;
header tcp_t tcp;
header udp_t udp;


/*******************************************************************
************************  P A R S E R ******************************
********************************************************************/

parser start {
	// start with ethernet parsing
	return parse_ethernet;
}

parser parse_ethernet {
	extract(ethernet);
	return select(latest.etherType) {
		TYPE_IPV4 : parse_ipv4;
		TYPE_IPV6 : parse_ipv6;
		TYPE_NSH : parse_nsh;
		TYPE_VLAN : parse_vlan;
		default: ingress;
	}
}

parser parse_vlan {
	extract(vlan);
	return select(latest.nextType) {
		TYPE_IPV4 : parse_ipv4;
		default: ingress;
	}
}

parser parse_ipv4 {
	extract(ipv4);
	return select(latest.protocol) {
		TYPE_TCP : parse_tcp;
		TYPE_UDP : parse_udp;
		default: ingress;
	}
}

parser parse_ipv6 {
	extract(ipv6);
	return ingress;
}

parser parse_nsh {
	extract(nsh);
	return select(latest.nextProto) {
		TYPE_NSH_IPV4 : parse_ipv4;
		TYPE_NSH_VLAN : parse_vlan;
		default: ingress;
	}
}

parser parse_tcp {
	extract(tcp);
	return ingress;
}

parser parse_udp {
	extract(udp);
	return ingress;
}

/*******************************************************************
************************  I N G R E S S ****************************
********************************************************************/

action sys_send_to_controller() {
    modify_field(ig_intr_md_for_tm.ucast_egress_port, CPU_PORT);
}
table sys_send_to_controller_apply {
    actions { sys_send_to_controller; }
    default_action : sys_send_to_controller();
    size : 0;
}

action sys_packet_drop() {
	drop();
}
table sys_drop_apply {
	actions { sys_packet_drop; }
	default_action : sys_packet_drop();
	size : 0;
}

action faas_init_metadata() {
	modify_field(meta.port_table_miss_flag, 0);
	modify_field(meta.port_table_miss_flag, 0);
	modify_field(meta.conn_table_miss_flag, 0);
	modify_field(meta.instance_table_miss_flag, 0);
}
table faas_init_metadata_apply {
    actions { faas_init_metadata; }
    default_action : faas_init_metadata;
    size : 0;
}

action faas_port_table_hit(egress_port) {
    modify_field(ig_intr_md_for_tm.ucast_egress_port, egress_port);
}

action faas_port_table_miss() {
    modify_field(meta.port_table_miss_flag, 1);
}

table faas_port_table {
    reads {
        ig_intr_md.ingress_port : exact;
    }
    actions {
        faas_port_table_hit;
        faas_port_table_miss;
    }
    default_action : faas_port_table_miss();
    size : PORT_TABLE_SIZE;
}

action faas_conn_table_hit(switch_port, dest_mac) {
    modify_field(meta.conn_table_miss_flag, 0);

    // Sets up the egress switch port.
    modify_field(ig_intr_md_for_tm.ucast_egress_port, switch_port);
    modify_field(ethernet.dstAddr, dest_mac);
}

action faas_conn_table_miss() {
    modify_field(meta.conn_table_miss_flag, 1);

    // Sends the first packet of the flow to the FaaS Controller.
    modify_field(meta.controller_flag, 1);
}

table faas_conn_table {
    reads {
        ipv4.srcAddr : exact;
        ipv4.dstAddr : exact;
        ipv4.protocol : exact;
        tcp.srcPort : exact;
        tcp.dstPort : exact;
    }
    actions {
        faas_conn_table_hit;
        faas_conn_table_miss;
    }
    default_action : faas_conn_table_miss();
    size : CONN_TABLE_SIZE;
}

action sys_init_metadata() {
	modify_field(meta.controller_flag, 0);
	modify_field(meta.drop_flag, 0);
}
table sys_init_metadata_apply {
	actions { sys_init_metadata; }
	default_action : sys_init_metadata;
	size:0;
}

control ingress {
	apply(sys_init_metadata_apply);
	apply(faas_init_metadata_apply);

	apply(faas_port_table);
	if (meta.port_table_miss_flag == 1) {
		apply(faas_conn_table);
	}

    if (meta.controller_flag == 1) {
    	apply(sys_send_to_controller_apply);
    }
	if (meta.drop_flag == 1) {
		apply(sys_drop_apply);
	}
} // end ingress


/*******************************************************************
************************  E G R E S S ****************************
********************************************************************/
control egress {
}
