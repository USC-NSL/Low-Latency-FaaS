package controller

import (
	"errors"
	"fmt"
	"sync"

	glog "github.com/golang/glog"
)

const (
	NIC_RX_QUEUE_LENGTH = 4096
	NIC_TX_QUEUE_LENGTH = 4096
	INVALID_CORE_ID     = -1
)

// The abstraction of minimal scheduling unit at each CPU core.
// |manager| manages NIC queues, memory buffers.
// |groupID| is the unique ID of the sGroup on a worker.
// |pcieIdx| is used to identify this sgroup.
// |instances| are NF instances within the scheduling group.
// |tids| is an array of all NF thread's IDs.
// |QueueLength, QueueCapacity| are the NIC queue information.
// |pktRateKpps| describes the observed traffic.
// |worker| is the worker node that the sGroup attached to. Set -1 when not attached.
// |coreID| is the core that the sGroup scheduled to.
// Note:
// 1. All Instances in |instances| is placed in a InsStartupPool;
// 2. If |tids| is empty, it means that one or more NF threadsl
// are not ready. CooperativeSched should NOT schedule these threads.
// 3. |maxRateKpps| is the profiling packet rate of running this SGroup
// on a single core.
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
	maxRateKpps      int
	worker           *Worker
	coreID           int
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
		maxRateKpps:      1000,
		worker:           w,
		coreID:           INVALID_CORE_ID,
	}

	isPrimary := "true"
	isIngress := "false"
	isEgress := "false"
	vPortIncIdx := 0
	vPortOutIdx := 0
	ins, err := w.createInstance("prim", pcieIdx, isPrimary, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
	if err != nil {
		// Fail to create the head instance. Cleanup..
		glog.Errorf("Failed to create Instance. %v", err)
		return nil
	}

	// Succeed.
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

// Destroys and removes all instances associaed with |sg|.
func (sg *SGroup) Reset() {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	for _, ins := range sg.instances {
		err := sg.worker.destroyInstance(ins)
		if err != nil {
			glog.Errorf("Failed to remove Pod[%s] in SGroup[%d]. %v", ins.funcType, sg.ID(), err)
		}
	}

	sg.instances = nil
	sg.tids = nil
}

func (sg *SGroup) AppendInstance(ins *Instance) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	ins.sg = sg
	sg.instances = append(sg.instances, ins)
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

// Returns true if all instances are ready to be scheduled.
func (sg *SGroup) IsReady() bool {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return len(sg.tids) > 0
}

// TODO (Jianfeng): trigger extra scaling operations.
func (sg *SGroup) UpdateTrafficInfo(qlen int, kpps int) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.incQueueLength = qlen
	sg.pktRateKpps = kpps
}

func (sg *SGroup) GetPktRate() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.pktRateKpps
}

func (sg *SGroup) GetLoad() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return 100 * sg.pktRateKpps / sg.maxRateKpps
}

func (sg *SGroup) UpdateCoreID(coreID int) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.coreID = coreID
}

// Migrates/Schedules a SGroup with |groupId| to core |coreId|.
func (sg *SGroup) attachSGroup(coreID int) error {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	if _, exists := sg.worker.cores[coreID]; !exists {
		return errors.New(fmt.Sprintf("core[%d] is not managed by FaaSController", coreID))
	}

	// Schedules |sg| on the new core via a gRPC request.
	if status, err := sg.worker.AttachChain(sg.tids, coreID); err != nil {
		return err
	} else if status.GetCode() != 0 {
		return errors.New(fmt.Sprintf("error from gRPC request AttachChain: %s", status.GetErrmsg()))
	}

	sg.coreID = coreID
	return nil
}

func (sg *SGroup) detachSGroup() error {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	if sg.coreID == INVALID_CORE_ID {
		return fmt.Errorf("SGroup[%d] is not running", sg.ID())
	}

	// Send gRPC to inform scheduler.
	if status, err := sg.worker.DetachChain(sg.tids, 0); err != nil {
		return err
	} else if status.GetCode() != 0 {
		return errors.New(fmt.Sprintf("error from gRPC request DetachChain: %s", status.GetErrmsg()))
	}

	return nil
}
