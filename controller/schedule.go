
package controller

import (
	"strconv"
)

// Functions for deciding how to allocate (when a new flow comes) and schedule instances (periodically).

// When a new flow comes to the system, needs to allocating instances to serve it according to its logical graph.
// Five-tuple (srcIp, scrPort, dstIP, dstPort, protocol) describes the new incoming flow in the system.
func (c *FaaSController) UpdateFlow(srcIP string, srcPort uint32, dstIP string, dstPort uint32, protocol uint32) error {
	// For a new flow, find out its logical sGroups(sub-chains) divisions.
	sGroups := c.sGroupsDivision(srcIP, srcPort, dstIP, dstPort, protocol)
	placement := c.schedule(sGroups)
	for i := range sGroups {
		worker := placement[i][0]
		coreId, _ := strconv.Atoi(placement[i][1])
		action := placement[i][2]
		if action == "allocate" {
			if err := c.workers[worker].scheduleSGroup(sGroups[i], coreId); err != nil {
				return err
			}
		}
	}
	// TODO: Send gRPC to notify ToR switch about routing information.
	return nil
}

// Give a flow five-tuple information, find out its logical chain divisions.
func (c *FaaSController) sGroupsDivision(srcIP string, srcPort uint32, dstIP string, dstPort uint32,
	protocol uint32) [][]string {
	return [][]string { {"a", "b", "c"} }
}

// For a group of |sGroups|, schedule them in the system.
// For each sGroup, return a tuple of <worker, coreIdx, action>.
// Action "place" means putting the sGroup to the existing chains.
// Action "allocate" means allocating a chain for the sGroup.
func (c *FaaSController) schedule(sGroups [][]string) [][3]string {
	placement := make([][3]string, len(sGroups))
	for i, sGroup := range sGroups {
		nodeName, coreIdx := c.findAvailableSGroup(sGroup)
		action := "place"
		if nodeName == "" {
			nodeName, coreIdx = c.allocateNewSGroup(sGroup)
			action = "allocate"
		}
		placement[i] = [3]string {nodeName, strconv.Itoa(coreIdx), action}
	}
	return placement
}

// Find existing sGroup to place the |sGroup|.
// Return worker |name| and core |coreIdx|.
// If not found, return "" and -1.
func (c *FaaSController) findAvailableSGroup(sGroup []string) (string, int) {
	for name, worker := range c.workers {
		for coreIdx, core := range worker.cores {
			for _, group := range core.sGroups {
				match := true
				for i, instance := range group.instances {
					if sGroup[i] != instance.funcType {
						match = false
						break
					}
				}
				if match {
					return name, coreIdx
				}
			}
		}
	}
	return "", -1
}

// Allocate |sGroup| to a new core.
// Return worker |name| and core |coreIdx|.
// If unavailable to allocate, return "" and -1.
func (c *FaaSController) allocateNewSGroup(sGroup []string) (string, int) {
	for name, worker := range c.workers {
		for coreIdx, core := range worker.cores {
			if len(core.sGroups) == 0 {
				return name, coreIdx
			}
		}
	}
	return "", -1
}
