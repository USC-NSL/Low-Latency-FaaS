package controller

import (
	"flag"
	"fmt"
	"math"
	"sync"
	"time"

	glog "github.com/golang/glog"
)

const (
	// The default NIC hardware rx/tx queue size.
	NIC_RX_QUEUE_LENGTH = 4096
	NIC_TX_QUEUE_LENGTH = 4096

	// Core indexes for idle chains
	IDLE_CHAINS_CORE_ID = 1
	INVALID_CORE_ID     = -1

	// The context switch time in CPU cycles
	CONTEXT_SWITCH_CYCLE_COST_PP = 5100

	// A SGroup turns ideleif it has been idle for at least 10 traffic samples.
	MIN_IDLE_DURATION = 10
)

// These are default PCIe devices in a host. Each of these devices has its
// own Dst MAC address listed in DefaultDstMACs (in the exactly same order).
var DefaultPCIeDevices = []string{
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

var DefaultDstMACs = []string{
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

var SupportQueueLength bool

func init() {
	flag.BoolVar(&SupportQueueLength, "SupportqueueLength", false, "Whether the PMD supports rte_eth_rx_queue_count function")
	flag.Parse()
	fmt.Printf("NIC supports queue count: %v\n", SupportQueueLength)
}

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
// |sumCycles| is the sum of all instances' cycle costs.
// |batchSize| is the batch size when executing NFs.
// |batchCount| is the target number of batches in one execution
// for all instances.
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
	idleSampleCnt    int
	instances        []*Instance
	tids             []int32
	sumCycles        int
	batchSize        int
	batchCount       int
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

func newSGroup(w *Worker, pcieIdx int) *SGroup {
	glog.Infof("Create a new SGroup at worker[%s]:pcie[%d]", w.name, pcieIdx)
	sg := SGroup{
		groupID:          pcieIdx,
		pcieIdx:          pcieIdx,
		isReady:          false,
		isActive:         false,
		isSched:          false,
		idleSampleCnt:    0,
		instances:        make([]*Instance, 0),
		tids:             make([]int32, 0),
		sumCycles:        0,
		batchSize:        32,
		batchCount:       1,
		incQueueLength:   0,
		incQueueCapacity: NIC_RX_QUEUE_LENGTH,
		outQueueLength:   0,
		outQueueCapacity: NIC_TX_QUEUE_LENGTH,
		pktRateKpps:      0,
		maxRateKpps:      800,
		worker:           w,
		coreID:           INVALID_CORE_ID,
	}

	isPrimary := "true"
	isIngress := "false"
	isEgress := "false"
	vPortIncIdx := 0
	vPortOutIdx := 0
	ins, err := w.createInstance("prim", 0, pcieIdx, isPrimary, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
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

	qLoad := sg.getQLoad()
	pLoad := sg.getPktLoad()

	info := "["
	for i, ins := range sg.instances {
		if i == 0 {
			info += fmt.Sprintf("%s", ins)
		} else {
			info += fmt.Sprintf(" -> %s", ins)
		}
	}
	info += fmt.Sprintf("]\n")
	info += fmt.Sprintf("    Info: id=%d, pcie=%s, core=%d\n", sg.groupID, sg.worker.pcie[sg.pcieIdx], sg.coreID)
	info += fmt.Sprintf("    Status: rdy=%v, active=%v, sched=%v\n", sg.isReady, sg.isActive, sg.isSched)
	info += fmt.Sprintf("    Performance: cycles=%d, batch=(size=%d, cnt=%d), (q=%d, qload=%d), (pps=%d kpps, pload=%d)", sg.sumCycles, sg.batchSize, sg.batchCount, sg.incQueueLength, qLoad, sg.pktRateKpps, pLoad)

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

// Appends a new Instance |ins| to the end of this SGroup |sg|.
func (sg *SGroup) AppendInstance(ins *Instance) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	ins.sg = sg
	sg.instances = append(sg.instances, ins)
}

// This function adjusts the batch parameters for all instances
// in this SGroup. It calculates the appropriate values based on
// the profiled NF cycle costs.
func (sg *SGroup) adjustBatchCount() {
	sumCycleCost := 0
	for _, ins := range sg.instances {
		sumCycleCost += ins.profiledCycle
	}
	nfCount := len(sg.instances)
	cnt := 5100 * float64(nfCount+1) / float64(1/0.95-1) / float64(sumCycleCost) / float64(sg.batchSize)
	sg.batchCount = int(math.Ceil(cnt))
	//sg.batchCount = int(math.Pow(2, math.Ceil(math.Log2(cnt))))

	for _, ins := range sg.instances {
		if err := ins.setBatch(sg.batchSize, sg.batchCount); err != nil {
			glog.Errorf("%v", err)
		}

		if ins.funcType == "bypass" {
			// All connections have been set up.
			if err := ins.setCycles(ins.profiledCycle); err != nil {
				glog.Errorf("%v", err)
			}
		}
	}
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
		glog.Infof("SGroup %d is ready. Notify the scheduler", sg.ID())

		w := sg.worker

		// Calls gRPC functions directly to avoid deadlocks.
		if _, err := w.SetupChain(sg.tids); err != nil {
			glog.Errorf("Failed to notify the scheduler. %s", err)
		}

		time.Sleep(100 * time.Millisecond)

		coreID := IDLE_CHAINS_CORE_ID
		if status, err := w.AttachChain(sg.tids, coreID); err != nil {
			glog.Errorf("Failed to attach SGroup[%d] on core #1. %s", sg.ID(), err)
		} else if status.GetCode() != 0 {
			glog.Errorf("AttachChain gRPC request errmsg: %s", status.GetErrmsg())
		}

		time.Sleep(100 * time.Millisecond)

		core, exists := w.cores[coreID]
		if !exists {
			glog.Errorf("Core[%d] not found", coreID)
		} else {
			core.addSGroup(sg)
		}

		time.Sleep(100 * time.Millisecond)

		sg.adjustBatchCount()

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

func (sg *SGroup) SetActive() {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.isActive = true
	sg.idleSampleCnt = 0
}

func (sg *SGroup) SetSched(isSched bool) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.isSched = isSched
}

// TODO (Jianfeng): trigger extra scaling operations.
// This function is called to update traffic-related parameters.
// * Updates the packet rate and queue length for SGroup |sg|.
// * Calculates the sum of cycle costs.
// * Estimates the max packet rate with the context switching overhead.
// * Marks |sg| active if there are packets in its NIC queue, and
// marks inactive if it has zero traffic rate and zero queue length.
func (sg *SGroup) UpdateTrafficInfo() {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	nfCount := len(sg.instances)
	if nfCount > 0 {
		sg.incQueueLength = sg.instances[0].getQlen()
		sg.pktRateKpps = sg.instances[0].getPktRate()

		sg.sumCycles = 0
		for _, ins := range sg.instances {
			sg.sumCycles += ins.getCycle()
		}

		// Calculates the max rate without context switching.
		if sg.sumCycles > 0 {
			sg.maxRateKpps = 1700000 / (sg.sumCycles + 5100*(nfCount+1)/(sg.batchSize*sg.batchCount))
		}
	}

	if sg.isActive {
		if sg.pktRateKpps == 0 {
			if !SupportQueueLength || (SupportQueueLength && sg.incQueueLength == 0) {
				sg.idleSampleCnt += 1
				if sg.isActive && sg.idleSampleCnt >= MIN_IDLE_DURATION {
					sg.isActive = false
				}
			}
		}
	} else {
		if SupportQueueLength && sg.incQueueLength > 0 {
			sg.isActive = true
		}
	}
}

func (sg *SGroup) GetCycles() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.sumCycles
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

	return sg.getPktLoad()
}

func (sg *SGroup) SetCoreID(coreID int) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	sg.coreID = coreID
}

func (sg *SGroup) GetCoreID() int {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	return sg.coreID
}

func (sg *SGroup) getQLoad() int {
	if sg.incQueueCapacity <= 0 {
		return 0
	}
	return 100 * sg.incQueueLength / sg.incQueueCapacity
}

// Returns the current packet rate devided by the estimated max
// packet rate (in percentage value).
// Note: |sg.maxRateKpps| has considered the context switching overhead.
// This is the old packet load formula: (no context switching overhead)
// CPU load = 100 * (packetRate * sumCycles) / (CPU Frequency)
// i.e. 100 * (sg.pktRateKpps * 1000 * sg.sumCycles) / (1700 * 1000,000)
// return (sg.pktRateKpps * sg.sumCycles) / 1700 * 10
func (sg *SGroup) getPktLoad() int {
	if sg.maxRateKpps == 0 {
		return 0
	}
	return 100 * sg.pktRateKpps / sg.maxRateKpps
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
	glog.Infof("SGroup[%d] runs on W %s Core[%d]", sg.ID(), sg.worker.name, coreID)
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
	glog.Infof("SGroup[%d] is detached on W %s Core[%d]", sg.ID(), sg.worker.name, sg.GetCoreID())
	return nil
}
