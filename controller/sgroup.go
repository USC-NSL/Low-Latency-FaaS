package controller

import (
	"fmt"
)

// The abstraction of minimal scheduling unit (or sub-chain) in each core.
// |instances| are the NF instances belonging to the scheduling group.
// |QueueLength, QueueCapacity| are the information of the queue before each sGroup.
// |traffic| describes the observed traffic in the sGroup.
// |worker| is the worker node that the sGroup assigned to.
// |coreId| is the core that the sGroup scheduled to.
type SGroup struct {
	instances        []*Instance
	incQueueLength   int
	incQueueCapacity int
	outQueueLength   int
	outQueueCapacity int
	traffic          int
	worker           *Worker
	coreId           int
}

func newSGroup(worker *Worker, coreId int, instances []*Instance) *SGroup {
	sGroup := SGroup{
		instances:        make([]*Instance, len(instances)),
		incQueueLength:   0,
		incQueueCapacity: 0,
		outQueueLength:   0,
		outQueueCapacity: 0,
		traffic:          0,
		worker:           worker,
		coreId:           coreId,
	}
	copy(sGroup.instances, instances)
	return &sGroup
}

func (s *SGroup) String() string {
	info := ""
	for idx, instance := range s.instances {
		if idx == 0 {
			info += fmt.Sprintf("%s", instance)
		} else {
			info += fmt.Sprintf("->%s", instance)
		}
	}
	return info
}
