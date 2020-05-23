package controller

import (
	"fmt"
	"sync"
	//"time"

	glog "github.com/golang/glog"
)

const (
	NIC_RX_QUEUE_LENGTH = 4096
	NIC_TX_QUEUE_LENGTH = 4096
	STARTUP_CORE_ID     = 1
	INVALID_CORE_ID     = -1
)

// The abstraction of minimal scheduling unit at each CPU core.
// |manager| manages NIC queues, memory buffers.
// |groupID| is the unique ID of the sGroup on a worker.
// |pcieIdx| is used to identify this sgroup.
// |isReady| is true if all instances are ready and detached on Core #1.
// |isActive| is true if this SGroup is serving traffic, i.e.
// |isSched| is true if this SGroup is scheduled on a core.
// packets are coming into the SGroup's NIC queue.
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
	isReady          bool
	isActive         bool
	isSched          bool
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
		isReady:          false,
		isActive:         false,
		isSched:          false,
		instances:        make([]*Instance, 0),
		tids:             make([]int32, 0),
		incQueueLength:   0,
		incQueueCapacity: NIC_RX_QUEUE_LENGTH,
		outQueueLength:   0,
		outQueueCapacity: NIC_TX_QUEUE_LENGTH,
		pktRateKpps:      0,
		maxRateKpps:      802,
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
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	info := "["
	for i, ins := range sg.instances {
		if i == 0 {
			info += fmt.Sprintf("%s", ins)
		} else {
			info += fmt.Sprintf(" -> %s", ins)
		}
	}
	info += fmt.Sprintf("] info: id=%d, pcie=%s, core=%d\n", sg.groupID, PCIeMappings[sg.pcieIdx], sg.coreID)
	info += fmt.Sprintf("    Status: rdy=%v, active=%v, sched=%v;   ", sg.isReady, sg.isActive, sg.isSched)
	info += fmt.Sprintf("Performance: q=%d, qload=%d, pps=%d kpps, pload=%d)", sg.incQueueLength, 100*sg.incQueueLength/sg.incQueueCapacity, sg.pktRateKpps, 100*sg.pktRateKpps/sg.maxRateKpps)

	return info
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

		w := sg.worker

		// Calls gRPC functions directly to avoid deadlocks.
		if _, err := w.SetupChain(sg.tids); err != nil {
			glog.Errorf("Failed to notify the scheduler. %s", err)
		}

		coreID := STARTUP_CORE_ID
		if status, err := w.AttachChain(sg.tids, coreID); err != nil {
			glog.Errorf("Failed to attach SGroup[%d] on core #1. %s", sg.ID(), err)
		} else if status.GetCode() != 0 {
			glog.Errorf("AttachChain gRPC request errmsg: %s", status.GetErrmsg())
		}
		/*
			if status, err := w.DetachChain(sg.tids, coreID); err != nil {
				glog.Errorf("Failed to detach SGroup[%d] on core #1. %s", sg.ID(), err)
			} else if status.GetCode() != 0 {
				glog.Errorf("DetachChain gRPC request errmsg: %s", status.GetErrmsg())
			}
		*/
		core, exists := w.cores[coreID]
		if !exists {
			glog.Errorf("Core[%d] not found", coreID)
		} else {
			core.attachSGroup(sg)
		}

		sg.coreID = 1
		sg.isReady = true
		sg.isSched = true
	}
}

// Returns true if all instances are ready to be scheduled.
func (sg *SGroup) IsReady() bool {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.isReady
}

func (sg *SGroup) IsActive() bool {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.isActive
}

func (sg *SGroup) IsSched() bool {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.isSched
}

// TODO (Jianfeng): trigger extra scaling operations.
// |sg| turns active if it has packets in its NIC queue, and turns
// inactive if it has zero traffic rate and zero queue length.
func (sg *SGroup) updateTrafficInfo() {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	if len(sg.instances) > 0 {
		sg.incQueueLength = sg.instances[0].getQlen()
		sg.pktRateKpps = sg.instances[0].getPktRate()
	}

	if sg.isActive {
		if sg.incQueueLength == 0 && sg.pktRateKpps == 0 {
			sg.isActive = false
		}
	} else {
		if sg.incQueueLength > 0 {
			sg.isActive = true
		}
	}
}

func (sg *SGroup) GetQlen() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.incQueueLength
}

func (sg *SGroup) GetQLoad() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return 100 * sg.incQueueLength / sg.incQueueCapacity
}

func (sg *SGroup) GetPktRate() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.pktRateKpps
}

func (sg *SGroup) GetPktLoad() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return 100 * sg.pktRateKpps / sg.maxRateKpps
}

func (sg *SGroup) SetCoreID(coreID int) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.coreID = coreID
}

func (sg *SGroup) SetSched(isSched bool) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.isSched = isSched
}

func (sg *SGroup) GetCoreID() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.coreID
}

// Migrates/Schedules a SGroup with |groupId| to core |coreId|.
func (sg *SGroup) attachSGroup(coreID int) error {
	if sg.GetCoreID() == coreID && sg.IsSched() {
		// Returns if |sg| is scheduled by |coreID| now.
		return nil
	}

	// Schedules |sg| on the new core with index |coreID|.
	if err := sg.worker.attachSGroup(sg, coreID); err != nil {
		return err
	}

	sg.SetCoreID(coreID)
	sg.SetSched(true)
	glog.Infof("SGroup[%d] runs on Core[%d]", sg.ID(), coreID)
	return nil
}

func (sg *SGroup) detachSGroup() error {
	if sg.GetCoreID() == INVALID_CORE_ID {
		return fmt.Errorf("SGroup[%d] is not running", sg.ID())
	}
	if !sg.IsSched() {
		// Returns if |sg| has already detached.
		return nil
	}

	// Detaches |sg| from its running Core.
	if err := sg.worker.detachSGroup(sg); err != nil {
		return err
	}

	sg.SetSched(false)
	glog.Infof("SGroup[%d] is detached on Core[%d]", sg.ID(), sg.GetCoreID())
	return nil
}
