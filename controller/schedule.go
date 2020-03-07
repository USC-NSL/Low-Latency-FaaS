
package controller

import (
)

// Functions for deciding how to allocate (when a new flow comes) and schedule instances (periodically).

// When a new flow comes to the system, needs to allocating instances to serve it according to its logical graph.
// Five-tuple (srcIp, scrPort, dstIP, dstPort, protocol) describes the new incoming flow in the system.
func (c *FaaSController) UpdateFlow(srcIP string, srcPort uint32, dstIP string, dstPort uint32, protocol uint32) error {
	return nil
}
