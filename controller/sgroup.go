package controller

import (
	"fmt"

	glog "github.com/golang/glog"
)

const (
	NIC_RX_QUEUE_LENGTH = 2048
	NIC_TX_QUEUE_LENGTH = 2048
)

// The abstraction of minimal scheduling unit at each CPU core.
// |ingress| manages NIC queues, memory buffers. |groupID| is
// a global index of |this| SGroup.
// |instances| are the NF instances belonging to the scheduling group.
// |QueueLength, QueueCapacity| are the NIC queue information.
// |pktRateKpps| describes the observed traffic.
// |worker| is the worker node that the sGroup attached to. Set -1 when not attached.
// |coreId| is the core that the sGroup scheduled to.
// |groupId| is the unique identifier of the sGroup on a worker. Technically, it is equal to the tid of its first NF instance.
// |tids| is an array of the tid of every instance in it.
// Both |groupId| and |pcieIdx| can be used to identify this sgroup.
type SGroup struct {
	ingress          *Instance
	groupId          int
	instances        []*Instance
	tids             []int32
	incQueueLength   int
	incQueueCapacity int
	outQueueLength   int
	outQueueCapacity int
	pktRateKpps      int
	worker           *Worker
	coreId           int
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

func newSGroup(w *Worker, pcieIdx int) *SGroup {
	glog.Infof("Create a new SGroup at worker[%s]:pcie[%d]", w.name, pcieIdx)
	isIngress := "true"
	isEgress := "false"
	vPortIncIdx := 0
	vPortOutIdx := 0

	ins := w.createInstance("none", pcieIdx, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
	// Fail to create the head instance. Cleanup..
	if ins == nil {
		return nil
	}
	w.SetUpThread(ins.tid)

	sg := SGroup{
		ingress:          ins,
		groupId:          ins.tid,
		instances:        make([]*Instance, 0),
		tids:             make([]int32, 0),
		incQueueLength:   0,
		incQueueCapacity: NIC_RX_QUEUE_LENGTH,
		outQueueLength:   0,
		outQueueCapacity: NIC_TX_QUEUE_LENGTH,
		pktRateKpps:      0,
		worker:           w,
		coreId:           -1,
		pcieIdx:          pcieIdx,
	}
	return &sg
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
	return info + fmt.Sprintf("](id=%d, pcie=%s, q=%d, pps=%d kpps)", s.groupId, PCIeMappings[s.pcieIdx], s.incQueueLength, s.pktRateKpps)
}
