
/* -*_ p4_14 -*- */
#include </root/jianfeng/tofino/intrinsic_metadata.p4>
#include </root/jianfeng/tofino/constants.p4>


#define TYPE_VLAN 0x0810
#define TYPE_NSH 0x894F
#define TYPE_NSH_IPV4 0x1
#define TYPE_NSH_VLAN 0x6
#define TYPE_IPV4 0x0800
#define TYPE_IPV6 0x0816
#define TYPE_TCP 0x06
#define TYPE_UDP 0x11

#define PORT_TABLE_SIZE 50

/*************************************************************************
*********************** H E A D E R S  ***********************************
*************************************************************************/


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
        controller_flag : 1;
        drop_flag : 1;
        port_table_miss_flag : 4;
        src_mac : 48;
        dst_mac : 48;
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


/*************************************************************************
************************  P A R S E R  **********************************
*************************************************************************/

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


/************************  I N G R E S S  **********************************/
action sys_pdrop() {
    drop();
}
table sys_drop_apply {
    actions { sys_pdrop; }
    default_action : sys_pdrop();
    size : 0;
}

action port_table_hit(egress_port) {
    modify_field(meta.src_mac, ethernet.srcAddr);
    modify_field(meta.dst_mac, ethernet.dstAddr);
    modify_field(ig_intr_md_for_tm.ucast_egress_port, egress_port);
}
action port_table_miss() {
    modify_field(meta.port_table_miss_flag, 1);
    modify_field(meta.drop_flag, 1);
}
table port_switching_table {
    reads {
        ig_intr_md.ingress_port : exact;
    }
    actions {
        port_table_hit;
        port_table_miss;
    }
    default_action : port_table_miss();
    size : PORT_TABLE_SIZE;
}

action mac_swap_act() {
    modify_field(ethernet.srcAddr, meta.dst_mac);
    modify_field(ethernet.dstAddr, meta.src_mac);
}
table mac_swap {
    actions {
        mac_swap_act;
    }
    default_action : mac_swap_act();
}

action sys_init_metadata() {
    modify_field(meta.controller_flag, 0);
    modify_field(meta.drop_flag, 0);
    modify_field(meta.port_table_miss_flag, 0);
}
table sys_init_metadata_apply {
    actions { sys_init_metadata; }
    default_action : sys_init_metadata;
    size:0;
}

control ingress {
    apply(sys_init_metadata_apply);

    apply(port_switching_table);

    if (meta.drop_flag == 1) {
        apply(sys_drop_apply);
    } else {
        apply(mac_swap);
    }
} // end ingress


/************************  E G R E S S  **********************************/
control egress {
}
