
package controller

import (
)

// The abstraction of minimal scheduling unit (or sub-chain) in each core.
// |instances| are the NF instances belong to the scheduling group.
// |QueueLength, QueueCapacity| are the information of the queue before each sGroup.
// |traffic| describes the observed traffic in the sGroup.
// |worker| is the worker node that the sGroup assigned to.
// |coreId| is the core that the sGroup scheduled to.
type SGroup struct {
	instances []*Instance
	incQueueLength int
	incQueueCapacity int
	outQueueLength int
	outQueueCapacity int
	traffic int
	worker *Worker
	coreId int
}

func newSGroup() *SGroup {
	sGroup := SGroup{
		instances: make([]*Instance, 0),
		incQueueLength: 0,
		incQueueCapacity: 0,
		outQueueLength: 0,
		outQueueCapacity: 0,
		traffic: 0,
		worker: nil,
		coreId: 0,
	}
	return &sGroup
}
