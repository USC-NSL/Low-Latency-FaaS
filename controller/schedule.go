package controller

import (
	"errors"
	"fmt"
)

// This is the place to implement resource allocation.
// First, when a flow arrives, |FaaSController| has to assign
// a sgroup to serve this flow;
// Second, NF chains are packed periodically based on real-time
// monitoring;

// When a new flow arrives, |FaaSController| picks a sgroup to
// serve. A flow is identified by its 5-tuple.
// TODO: Complete the reasons for returning errors.
func (c *FaaSController) UpdateFlow(srcIP string, srcPort uint32, dstIP string, dstPort uint32, protocol uint32) (string, error) {
	// For a new flow, find out its logical sGroup(sub-chain) divisions.
	funcTypes := c.sGroupsDivision(srcIP, srcPort, dstIP, dstPort, protocol)

	if sGroup := c.findRunnableSGroup(funcTypes); sGroup != nil {
		return dmacMappings[sGroup.pcieIdx], nil
	}

	if sGroup, coreId := c.findFreeSGroup(funcTypes); sGroup != nil {
		sGroup.worker.attachSGroup(sGroup, coreId)
		return dmacMappings[sGroup.pcieIdx], nil
	}

	sGroup, coreId, err := c.FindCoreToServeSGroup(funcTypes)
	if err != nil {
		return "", err
	}
	sGroup.worker.attachSGroup(sGroup, coreId)
	// TODO: Packet loss due to busy waiting
	return dmacMappings[sGroup.pcieIdx], nil
}

// Give a flow five-tuple information, find out its logical
// chain divisions.
func (c *FaaSController) sGroupsDivision(srcIP string, srcPort uint32, dstIP string, dstPort uint32,
	protocol uint32) []string {
	return []string{"a", "b", "c"}
}

// Find runnable |sGroup| on a core to place the logical chain with |funcTypes|.
// If not found, return nil.
func (c *FaaSController) findRunnableSGroup(funcTypes []string) *SGroup {
	for _, worker := range c.workers {
		sGroup := worker.findRunnableSGroup(funcTypes)
		if sGroup != nil {
			return sGroup
		}
	}
	return nil
}

// Find free |sGroup| in freeSGroups to place the logical chain with |funcTypes|.
// Also, an available |core| is required on the worker.
// Return (sGroup, coreId).
// If not found, return (nil, -1).
func (c *FaaSController) findFreeSGroup(funcTypes []string) (*SGroup, int) {
	for _, worker := range c.workers {
		sGroup := worker.findFreeSGroup(funcTypes)
		if sGroup != nil {
			// TODO: Adjust core selection.
			if coreId := worker.findAvailableCore(); coreId != -1 {
				return sGroup, coreId
			}
		}
	}
	return nil, -1
}

// Find available worker to create a new |sGroup|.
// Return (sGroup, coreId, error).
// If unavailable to allocate, return (nil, -1, error).
func (c *FaaSController) FindCoreToServeSGroup(funcTypes []string) (*SGroup, int, error) {
	// TODO: Modify allocating strategy
	for _, worker := range c.workers {
		// TODO: Confirm there is enough pci for creating container.
		if coreId := worker.findAvailableCore(); coreId != -1 {
			if sGroup, err := worker.createSGroup(funcTypes); err != nil {
				return nil, -1, err
			} else {
				return sGroup, coreId, err
			}
		}
	}
	return nil, -1, errors.New(fmt.Sprintf("could not find available worker"))
}
