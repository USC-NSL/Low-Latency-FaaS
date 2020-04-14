package controller

import (
	"fmt"
)

// The abstraction of minimal scheduling unit (or sub-chain) in each core.
// |instances| are the NF instances belonging to the scheduling group.
// |QueueLength, QueueCapacity| are the information of the queue before each sGroup.
// |traffic| describes the observed traffic in the sGroup.
// |worker| is the worker node that the sGroup attached to. Set -1 when not attached.
// |coreId| is the core that the sGroup scheduled to.
// |groupId| is the unique identifier of the sGroup on a worker. Technically, it is equal to the tid of its first NF instance.
// |tids| is an array of the tid of every instance in it.
type SGroup struct {
	instances        []*Instance
	incQueueLength   int
	incQueueCapacity int
	outQueueLength   int
	outQueueCapacity int
	traffic          int
	worker           *Worker
	coreId           int
	groupId          int
	tids             []int32
	pcieIdx          int
}

var PCIeMappings = []string{
	"5e:02.0",
	"5e:02.1",
	"5e:02.2",
	"5e:02.3",
	"5e:02.4",
	"5e:02.5",
	"5e:02.6",
	"5e:02.7",
	"5e:03.0",
	"5e:03.1",
	"5e:03.2",
	"5e:03.3",
	"5e:03.4",
	"5e:03.5",
	"5e:03.6",
}

var dmacMappings = []string{
	"00:00:00:00:00:01",
	"00:00:00:00:00:02",
	"00:00:00:00:00:03",
	"00:00:00:00:00:04",
	"00:00:00:00:00:05",
	"00:00:00:00:00:06",
	"00:00:00:00:00:07",
	"00:00:00:00:00:08",
	"00:00:00:00:00:09",
	"00:00:00:00:00:10",
	"00:00:00:00:00:11",
	"00:00:00:00:00:12",
	"00:00:00:00:00:13",
	"00:00:00:00:00:14",
	"00:00:00:00:00:15",
}

func newSGroup(worker *Worker, instances []*Instance, pcieIdx int) *SGroup {
	sGroup := SGroup{
		instances:        make([]*Instance, len(instances)),
		incQueueLength:   0,
		incQueueCapacity: 0,
		outQueueLength:   0,
		outQueueCapacity: 0,
		traffic:          0,
		worker:           worker,
		coreId:           -1,
		groupId:          instances[0].tid,
		tids:             make([]int32, len(instances)),
		pcieIdx:          pcieIdx,
	}
	copy(sGroup.instances, instances)
	for i, instance := range instances {
		sGroup.tids[i] = int32(instance.tid)
	}
	return &sGroup
}

func (s *SGroup) String() string {
	info := "["
	for idx, instance := range s.instances {
		if idx == 0 {
			info += fmt.Sprintf("%s", instance)
		} else {
			info += fmt.Sprintf("->%s", instance)
		}
	}
	return info + fmt.Sprintf("](id=%d,pcie=%s)", s.groupId, PCIeMappings[s.pcieIdx])
}

func (s *SGroup) match(funcTypes []string) bool {
	if len(s.instances) != len(funcTypes) {
		return false
	}
	for i, instance := range s.instances {
		if instance.funcType != funcTypes[i] {
			return false
		}
	}
	return true
}
