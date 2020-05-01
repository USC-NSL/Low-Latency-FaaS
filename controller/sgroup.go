package controller

import (
	"fmt"
	"sync"

	glog "github.com/golang/glog"
)

const (
	NIC_RX_QUEUE_LENGTH = 4096
	NIC_TX_QUEUE_LENGTH = 4096
)

// The abstraction of minimal scheduling unit at each CPU core.
// |manager| manages NIC queues, memory buffers.
// |pcieIdx| is used to identify this sgroup.
// |instances| are NF instances within the scheduling group.
// |QueueLength, QueueCapacity| are the NIC queue information.
// |pktRateKpps| describes the observed traffic.
// |worker| is the worker node that the sGroup attached to. Set -1 when not attached.
// |coreId| is the core that the sGroup scheduled to.
// |groupID| is the unique identifier of the sGroup on a worker. Technically, it is equal to the tid of its first NF instance.
// |tids| is an array of the tid of every instance in it.
type SGroup struct {
	manager          *Instance
	groupID          int
	pcieIdx          int
	instances        []*Instance
	tids             []int32
	incQueueLength   int
	incQueueCapacity int
	outQueueLength   int
	outQueueCapacity int
	pktRateKpps      int
	worker           *Worker
	coreId           int
	mutex            sync.Mutex
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
	sg := SGroup{
		groupID:          pcieIdx,
		pcieIdx:          pcieIdx,
		instances:        make([]*Instance, 0),
		tids:             make([]int32, 0),
		incQueueLength:   0,
		incQueueCapacity: NIC_RX_QUEUE_LENGTH,
		outQueueLength:   0,
		outQueueCapacity: NIC_TX_QUEUE_LENGTH,
		pktRateKpps:      0,
		worker:           w,
		coreId:           -1,
	}

	isPrimary := "true"
	isIngress := "false"
	isEgress := "false"
	vPortIncIdx := 0
	vPortOutIdx := 0
	ins, err := w.createInstance("prim", pcieIdx, isPrimary, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
	// Fail to create the head instance. Cleanup..
	if err != nil {
		return nil
	}

	sg.manager = ins
	return &sg
}

func (sg *SGroup) String() string {
	info := "["
	for i, ins := range sg.instances {
		if i == 0 {
			info += fmt.Sprintf("%s", ins)
		} else {
			info += fmt.Sprintf("->%s", ins)
		}
	}
	return info + fmt.Sprintf("](id=%d, pcie=%s, q=%d, pps=%d kpps)", sg.groupID, PCIeMappings[sg.pcieIdx], sg.incQueueLength, sg.pktRateKpps)
}

func (sg *SGroup) ID() int {
	return sg.pcieIdx
}

func (sg *SGroup) reset() {
	sg.instances = nil
	sg.tids = nil
}

func (sg *SGroup) UpdateTID(port int, tid int) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	ready := true
	for _, ins := range sg.instances {
		if ins.ID() == port {
			ins.tid = tid
		}
		if ins.tid == kUninitializedTid {
			ready = false
		}
	}

	if ready {
		for _, ins := range sg.instances {
			sg.tids = append(sg.tids, int32(ins.tid))
		}

		// Sends gRPC request to inform the scheduler.
		glog.Infof("SGroup[%d] is ready. Notify the scheduler", sg.ID())
		_, err := sg.worker.SetupChain(sg.tids)
		if err != nil {
			glog.Errorf("Failed to notify the scheduler. %s", err)
		}
	}
}
