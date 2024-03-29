package controller

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	kubectl "github.com/USC-NSL/Low-Latency-FaaS/kubectl"
	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
	glog "github.com/golang/glog"
)

const (
	// The default Redis server's port for the control-plane.
	kControlPlaneRedisPass = "faas-nfv-cool"
	kControlPlaneRedisPort = 6379

	// The default per-worker vSwitch (BESS) gRPC port.
	vSwitchPort = 10514

	// The default per-worker CoopSched gRCP port.
	schedulerPort = 10515
)

// The abstraction of a worker node.
// |VSwitchGRPCHandler| and |SchedulerGRPCHandler| are functions
// to handle gRPC requests to vSwitch and scheduler on the worker.
// |name| is the name of the node in kubernetes.
// |ip| is the ip address of the worker node.
// |vSwitchPort| is BESS gRPC port on host (e.g. FlowGen).
// |cores| maps real core numbers to CPU cores.
// |sgroups| contains all deployed sgroups on the worker.
// |freeSGroups| are free sGroups not pinned to any core yet (but in memory).
// |instancePortPool| manages ports taken by instances on the node.
// This is to prevent conflicts on host TCP ports.
// |pciePool| manages pcie port taken by sGroup on the node.
// |insStartupPool| is a pool for instances that are on start-up.
// |op| is a channle to FreeSGroup maintainer(go routine).
// |wg| is a waiting group for all go routines of this worker.
// |sgMutex| only protects |sgroups| and |freeSGroups|.
type Worker struct {
	grpc.VSwitchGRPCHandler
	grpc.SchedulerGRPCHandler
	name             string
	ip               string
	pcie             []string
	switchPort       uint32
	sched            *Instance
	cores            map[int]*Core
	sgroups          SGroupSlice
	sgroupConns      []int
	sgroupTarget     int
	upMutex          sync.Mutex
	freeSGroups      SGroupSlice
	instancePortPool *utils.IndexPool
	pciePool         *utils.IndexPool
	insStartupPool   *InstancePool
	bgTraffic        bool
	op               chan FaaSOP
	schedOp          chan FaaSOP
	wg               sync.WaitGroup
	sgMutex          sync.Mutex
}

func NewWorker(name string, ip string, coreNumOffset int, coreNum int, pcie []string, switchPortNum uint32) *Worker {
	perWorkerPCIeDevices := make([]string, 0)
	if len(pcie) > 0 {
		perWorkerPCIeDevices = pcie
	} else {
		perWorkerPCIeDevices = DefaultPCIeDevices
	}

	// Ports taken by instances are between [50052, 51051]
	w := Worker{
		name:             name,
		ip:               ip,
		pcie:             perWorkerPCIeDevices,
		switchPort:       uint32(switchPortNum),
		cores:            make(map[int]*Core),
		sgroups:          make([]*SGroup, 0),
		sgroupConns:      make([]int, 0),
		sgroupTarget:     int(0),
		freeSGroups:      make([]*SGroup, 0),
		instancePortPool: utils.NewIndexPool(50052, 1000),
		pciePool:         utils.NewIndexPool(0, len(perWorkerPCIeDevices)),
		insStartupPool:   NewInstancePool(),
		bgTraffic:        false,
		op:               make(chan FaaSOP, 64),
		schedOp:          make(chan FaaSOP, 64),
	}

	// coreId is ranged between [coreNumOffset, coreNumOffset + coreNum)
	for i := 0; i < coreNum; i++ {
		coreID := i + coreNumOffset
		w.cores[coreID] = NewCore(coreID)
	}

	// TODO(Zhuojin): remove VSwitchGRPCHandler.
	//if err := w.VSwitchGRPCHandler.EstablishConnection(fmt.Sprintf("%s:%d", ip, vSwitchPort)); err != nil {
	//	fmt.Println("Fail to connect with vSwitch: " + err.Error())
	//}

	return &w
}

func (w *Worker) String() string {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	info := fmt.Sprintf("Worker [%s] at %s \n Core:", w.name, w.ip)

	coreIDs := []int{}
	for coreID := range w.cores {
		coreIDs = append(coreIDs, coreID)
	}

	sort.Ints(coreIDs)
	for _, id := range coreIDs {
		info += fmt.Sprintf("\n  %s", w.cores[id])
	}

	info += "\n SGroups:"
	for id := 0; id < len(w.sgroups); id++ {
		for _, sg := range w.sgroups {
			if sg.groupID == id {
				info += fmt.Sprintf("\n  %s", sg)
				break
			}
		}
	}

	info += fmt.Sprintf("\n %d remaining free SGroups", len(w.freeSGroups))

	return info + "\n"
}

func (w *Worker) GetPktLoad() int {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	sumLoad := 0
	for _, sg := range w.sgroups {
		sumLoad += sg.GetPktLoad()
	}
	return sumLoad
}

// Bring up background threads for each worker.
// (1) FreeSGroupFactory: the background thread for creating new free SGs;
// (2) SchedulerLoop: the per-worker thread monitors CPU and traffic loads;
func (w *Worker) faasInit() {
	// Starts a background routine for maintaining |freeSGroups|
	w.wg.Add(2)
	go w.RunFreeSGroupFactory(w.op)
	go w.ScheduleLoop()

	glog.Infof("FaaS Worker[%s] is up.", w.name)
}

// Metron does not run per-worker monitoring functions.
func (w *Worker) metronInit() {
	w.wg.Add(1)
	go w.RunFreeSGroupFactory(w.op)

	glog.Infof("Metron Worker[%s] is up.", w.name)
}

func (w *Worker) createSched() error {
	// |IndexPool| is thread-safe.
	port := w.instancePortPool.GetNextAvailable()
	podName, err := kubectl.K8sHandler.CreateSchedDeployment(w.name, port)
	if err != nil {
		return err
	}
	w.sched = newInstance("sched", false, false, 0, w.ip, port, podName)

	schedAddr := fmt.Sprintf("%s:%d", w.ip, port)
	start := time.Now()
	for time.Now().Unix()-start.Unix() < 30 {
		err := w.SchedulerGRPCHandler.EstablishConnection(schedAddr)
		if err == nil {
			break
		}

		// Backoff..
		time.Sleep(500 * time.Millisecond)
	}

	if !w.SchedulerGRPCHandler.IsConnEstablished() {
		return fmt.Errorf("Fail to connect with scheduler[%s].", schedAddr)
	}

	glog.Errorf("Connect with scheduler[%s].", schedAddr)
	return nil
}

// Creates an NF instance |ins| with type |funcType|. This instance
// is stored in the worker's |insStartupPool|. The controller waits
// for its |tid| sent from its NF thread.
func (w *Worker) createInstance(nfTypes []string, cycleCost int, pcieIdx int, coreID int, isPrimary bool, isIngress bool, isEgress bool, vPortIncIdx int, vPortOutIdx int) (*Instance, error) {
	// Both |IndexPool| and |InstancePool| are thread-safe types.
	port := w.instancePortPool.GetNextAvailable()
	podName, err := kubectl.K8sHandler.CreateDeployment(
		w.name, nfTypes, port, w.pcie[pcieIdx], coreID,
		isPrimary, isIngress, isEgress,
		vPortIncIdx, vPortOutIdx)
	if err != nil {
		w.instancePortPool.Free(port)
		return nil, err
	}

	ins := newInstance(strings.Join(nfTypes, ","), isIngress, isEgress, cycleCost, w.ip, port, podName)
	if !isPrimary {
		w.insStartupPool.add(ins)
	}

	// Succeed
	return ins, nil
}

// Destroys an NF |ins|. Note: this function only gets called
// when freeing a sGroup.
func (w *Worker) destroyInstance(ins *Instance) error {
	if ins == nil {
		return nil
	}

	err := kubectl.K8sHandler.DeleteDeployment(ins.podName)
	if err != nil {
		return err
	}

	// Succeed
	w.instancePortPool.Free(ins.port)
	return nil
}

func (w *Worker) createAllFreeSGroups() {
	for i := 0; i < 2; i++ {
		//for i := 0; i < w.pciePool.Size(); i++ {
		w.op <- FREE_SGROUP
	}
}

// Destorys and removes all free SGroups in |w.freeSGroups|. This
// function waits for all free SGroups to finish the cleanup process.
// Otherwise, the main thread may return before all go (cleanup)
// routines proceed successfully. Hence, some of free SGroups may not
// get removed up.
func (w *Worker) destroyAllFreeSGroups() {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(w.freeSGroups))

	for len(w.freeSGroups) > 0 {
		idx := len(w.freeSGroups)
		sg := w.freeSGroups[idx-1]
		w.freeSGroups = w.freeSGroups[:(idx - 1)]

		go w.destroyFreeSGroup(sg, &wg)
	}

	wg.Wait()
}

func (w *Worker) countFreeSGroups() int {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	return len(w.freeSGroups)
}

// Returns a free sGroup |sg| in |w.freeSGroups|.
// |sg| is removed from |w|'s freeSGroups. The caller acquires |sg|.
// No one else should acquire |sg| at the same time.
// Returns nil if |w.freeSGroups| is empty.
func (w *Worker) getFreeSGroup() *SGroup {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	n := len(w.freeSGroups)
	if n >= 1 {
		sg := w.freeSGroups[n-1]
		// Removes |sg| from w.freeSGroups.
		w.freeSGroups = w.freeSGroups[:(n - 1)]
		return sg
	}

	return nil
}

// Destorys and removes all SGroups in |w.sgroups|.
func (w *Worker) destroyAllSGroups() {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	for len(w.sgroups) > 0 {
		idx := len(w.sgroups)
		sg := w.sgroups[idx-1]
		w.sgroups = w.sgroups[:(idx - 1)]

		sg.Reset()
		w.freeSGroups = append(w.freeSGroups, sg)
	}
}

// Destroys a SGroup |sg|.
// Note: Unable to destroy a SGroup which is currently attached to a core.
func (w *Worker) destroySGroup(sg *SGroup) error {
	sg.Reset()

	w.sgMutex.Lock()
	w.freeSGroups = append(w.freeSGroups, sg)
	w.sgMutex.Unlock()

	return nil
}

func (w *Worker) countPendingSGroups() int {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	cnt := 0
	for _, sg := range w.sgroups {
		if sg.IsReady() {
			cnt += 1
		}
	}
	return cnt
}

func (w *Worker) isAllSGroupsConnected() bool {
	w.upMutex.Lock()
	defer w.upMutex.Unlock()

	glog.Infof("%s: %v, target:%d", w.name, w.sgroupConns, w.sgroupTarget)
	return len(w.sgroupConns) == w.sgroupTarget
}

func (w *Worker) getSGroup(groupID int) *SGroup {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	for _, sg := range w.sgroups {
		if sg.ID() == groupID {
			return sg
		}
	}

	return nil
}

// Migrates/Schedules a SGroup with |groupId| to core |coreId|.
func (w *Worker) attachSGroup(sg *SGroup, coreID int) error {
	// Removes |sg| from its previous core.
	prevCoreID := sg.GetCoreID()
	if prevCoreID != kFaaSInvalidCoreID {
		prevCore, exists := w.cores[prevCoreID]
		if !exists {
			return errors.New(fmt.Sprintf("Core[%d] not found", prevCoreID))
		}
		prevCore.removeSGroup(sg)
	}

	// Sends gRPC to inform scheduler.
	if status, err := w.AttachChain(sg.tids, coreID); err != nil {
		return err
	} else if status.GetCode() != 0 {
		return errors.New(fmt.Sprintf("AttachChain gRPC request errmsg: %s", status.GetErrmsg()))
	}

	// Appends |sg| to |core|.
	core, exists := w.cores[coreID]
	if !exists {
		return errors.New(fmt.Sprintf("Core[%d] not found", coreID))
	}
	core.addSGroup(sg)

	return nil
}

// Detaches a SGroup, indexed by |groupID|, on its running core.
// The SGroup is still pinned to its original running core, but won't
// get executed.
func (w *Worker) detachSGroup(sg *SGroup) error {
	// Send gRPC to inform scheduler.
	if status, err := w.DetachChain(sg.tids, 0); err != nil {
		return err
	} else if status.GetCode() != 0 {
		return errors.New(fmt.Sprintf("DetachChain gRPC request errmsg: %s", status.GetErrmsg()))
	}

	return nil
}

// Unschedule all NF threads and shutdown CooperativeSched.
// Destroy all SGroups (Instances), FreeSGroups on worker |w|.
func (w *Worker) Close() error {
	errmsg := []string{}

	if controllerOption == "faas" {
		// Shutdowns and waits for all background go routines.
		w.op <- SHUTDOWN
		w.schedOp <- SHUTDOWN
		w.wg.Wait()

		// Sends a gRPC request to turn CoopSched off.
		res, err := w.KillSched()
		if err != nil {
			msg := "Connection failed when killing CooperativeSched"
			errmsg = append(errmsg, msg)
		} else if res.GetError().GetCode() != 0 {
			msg := fmt.Sprintf("Failed to kill CooperativeSched. Reason: %s", res.GetError().GetErrmsg())
			errmsg = append(errmsg, msg)
		}

		w.destroyInstance(w.sched)

		// Cleans up SGroups and free SGroups.
		w.destroyAllSGroups()
		w.destroyAllFreeSGroups()
	} else if controllerOption == "metron" {
		w.op <- SHUTDOWN
		w.wg.Wait()

		// Cleans up SGroups and free SGroups.
		w.destroyAllSGroups()
		w.destroyAllFreeSGroups()
	}

	if len(errmsg) > 0 {
		return errors.New(strings.Join(errmsg, ""))
	}
	fmt.Printf("worker[%s] is cleaned up\n", w.name)
	return nil
}

func (w *Worker) setCycles(port int, cyclesPerPacket int) error {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	ins := w.insStartupPool.get(port)
	if ins == nil || ins.sg == nil {
		return errors.New(fmt.Sprintf("Cannot find instance with port %d on %s", port, w.name))
	}
	return ins.setCycles(cyclesPerPacket)
}

func (w *Worker) setBatch(port int, batchSize int, batchNumber int) error {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	ins := w.insStartupPool.get(port)
	if ins == nil || ins.sg == nil {
		return errors.New(fmt.Sprintf("Cannot find instance with port %d on %s", port, w.name))
	}
	return ins.setBatch(batchSize, batchNumber)
}
