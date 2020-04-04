package controller

// This is the place to implement resource allocation.
// First, when a flow arrives, |FaaSController| has to assign
// a sgroup to serve this flow;
// Second, NF chains are packed periodically based on real-time
// monitoring;

// When a new flow arrives, |FaaSController| picks a sgroup to
// serve. A flow is identified by its 5-tuple.
// TODO: Complete the reasons for returning errors.
func (c *FaaSController) UpdateFlow(srcIP string, srcPort uint32, dstIP string, dstPort uint32, protocol uint32) error {
	return nil
}
