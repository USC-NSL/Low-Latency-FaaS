package controller

type Flowlet interface {
	Match(srcIP string, dstIP string, srcPort uint32, dstPort uint32, proto uint32) bool
}

type flowlet struct {
	srcIP   string
	dstIP   string
	srcPort uint32
	dstPort uint32
	proto   uint32
}

func (f *flowlet) Match(srcIP string, dstIP string, srcPort uint32, dstPort uint32, proto uint32) bool {
	return (f.srcIP == "" || f.srcIP == srcIP) &&
		(f.dstIP == "0" || f.dstIP == dstIP) &&
		(f.srcPort == 0 || f.srcPort == srcPort) &&
		(f.dstPort == 0 || f.dstPort == dstPort)
}
