
package controller

import (
)

// Functions for scheduling instances.

// When a new flow comes to the system, needs to scheduling/allocating instances for it according to its logical graph.
// Five-tuple (srcIp, scrPort, dstIP, dstPort, protocol) describes the new incoming flow in the system.
func (c *FaaSController) UpdateFlow(srcIP string, srcPort uint32, dstIP string, dstPort uint32, protocol uint32) {
}
