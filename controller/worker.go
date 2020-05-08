package controller

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	kubectl "github.com/USC-NSL/Low-Latency-FaaS/kubectl"
	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
	glog "github.com/golang/glog"
)

const (
	vSwitchPort   = 10514
	schedulerPort = 10515
)

// The abstraction of a worker node.
// |VSwitchGRPCHandler| and |SchedulerGRPCHandler| are functions
// to handle gRPC requests to vSwitch and scheduler on the worker.
// |name| is the name of the node in kubernetes.
// |ip| is the ip address of the worker node.
// |vSwitchPort| is BESS gRPC port on host (e.g. FlowGen).
// |schedulerPort| is Cooperativesched gRCP port on host.
// |cores| maps real core numbers to CPU cores.
// |sgroups| contains all deployed sgroups on the worker.
// |freeSGroups| are free sGroups not pinned to any core yet (but in memory).
// |instancePortPool| manages ports taken by instances on the node.
// This is to prevent conflicts on host TCP ports.
// |pciePool| manages pcie port taken by sGroup on the node.
// |insStartupPool| is a pool for instances that are on start-up.
// |op| is a channle to FreeSGroup maintainer(go routine).
// |wg| is a waiting group for all go routines of this worker.
// |sgMutex| is for protecting |sgroups| and |freeSGroups|.
type Worker struct {
	grpc.VSwitchGRPCHandler
	grpc.SchedulerGRPCHandler
	name             string
	ip               string
	sched            *Instance
	cores            map[int]*Core
	sgroups          SGroupSlice
	freeSGroups      SGroupSlice
	instancePortPool *utils.IndexPool
	pciePool         *utils.IndexPool
	insStartupPool   *InstancePool
	op               chan FaaSOP
	schedOp          chan FaaSOP
	wg               sync.WaitGroup
	sgMutex          sync.Mutex
}

func NewWorker(name string, ip string, coreNumOffset int, coreNum int) *Worker {
	// Ports taken by instances are between [50052, 51051]
	w := Worker{
		name:             name,
		ip:               ip,
		cores:            make(map[int]*Core),
		sgroups:          make([]*SGroup, 0),
		freeSGroups:      make([]*SGroup, 0),
		instancePortPool: utils.NewIndexPool(50052, 1000),
		pciePool:         utils.NewIndexPool(0, len(PCIeMappings)),
		insStartupPool:   NewInstancePool(),
		op:               make(chan FaaSOP, 64),
		schedOp:          make(chan FaaSOP, 64),
	}

	// coreId is ranged between [coreNumOffset, coreNumOffset + coreNum)
	for i := 0; i < coreNum; i++ {
		w.cores[i+coreNumOffset] = newCore()
	}

	// Starts a background routine for maintaining |freeSGroups|
	w.wg.Add(2)
	go w.CreateFreeSGroups(w.op)
	go w.ScheduleLoop()

	// TODO(Zhuojin): remove VSwitchGRPCHandler.
	//if err := w.VSwitchGRPCHandler.EstablishConnection(fmt.Sprintf("%s:%d", ip, vSwitchPort)); err != nil {
	//	fmt.Println("Fail to connect with vSwitch: " + err.Error())
	//}

	glog.Infof("Worker[%s] is up.", w.name)

	return &w
}

func (w *Worker) String() string {
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	info := fmt.Sprintf("Worker [%s] at %s \n Core:", w.name, w.ip)
	for coreId, core := range w.cores {
		info += fmt.Sprintf("\n  %d %s", coreId, core)
	}

	info += "\n SGroups:"
	for _, sg := range w.sgroups {
		info += fmt.Sprintf("\n  %s", sg)
	}

	info += fmt.Sprintf("\n %d remaining free SGroups", len(w.freeSGroups))

	return info + "\n"
}

func (w *Worker) createSched() error {
	// |IndexPool| is thread-safe.
	port := w.instancePortPool.GetNextAvailable()
	podName, err := kubectl.K8sHandler.CreateSchedDeployment(w.name, port)
	if err != nil {
		return err
	}
	w.sched = newInstance("sched", w.ip, port, podName)

	schedAddr := fmt.Sprintf("%s:%d", w.ip, port)
	start := time.Now()
	for time.Now().Unix()-start.Unix() < 10 {
		err := w.SchedulerGRPCHandler.EstablishConnection(schedAddr)
		if err == nil {
			break
		}

		// Backoff..
		time.Sleep(1000 * time.Millisecond)
	}

	if !w.SchedulerGRPCHandler.IsConnEstablished() {
		return fmt.Errorf("Fail to connect with scheduler[%s].", schedAddr)
	}

	glog.Errorf("Connect with scheduler[%s].", schedAddr)
	return nil
}

// Creates an NF instance |ins| with type |funcType|. After |ins|
// is created and starts up, the instance is temporarily stored
// in the worker's |insStartupPool| and waits for its |tid| sent
// from its NF thread.
func (w *Worker) createInstance(funcType string, pcieIdx int, isPrimary string, isIngress string, isEgress string, vPortIncIdx int, vPortOutIdx int) (*Instance, error) {
	// Both |IndexPool| and |InstancePool| are thread-safe types.
	port := w.instancePortPool.GetNextAvailable()
	podName, err := kubectl.K8sHandler.CreateDeployment(w.name, funcType, port, PCIeMappings[pcieIdx], isPrimary, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
	if err != nil {
		w.instancePortPool.Free(port)
		return nil, err
	}

	ins := newInstance(funcType, w.ip, port, podName)
	if isPrimary != "true" {
		w.insStartupPool.add(ins)
	}

	// Succeed
	return ins, nil
}

// Destroys an NF |ins|. Note: this function only gets called
// when freeing a sGroup.
func (w *Worker) destroyInstance(ins *Instance) error {
	err := kubectl.K8sHandler.DeleteDeployment(ins.podName)
	if err != nil {
		return err
	}

	// Succeed
	w.instancePortPool.Free(ins.port)
	return nil
}

func (w *Worker) createAllFreeSGroups() {
	//for i := 0; i < w.pciePool.Size(); i++ {
	for i := 0; i < 4; i++ {
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
func (w *Worker) attachSGroup(groupID int, coreID int) error {
	if _, exists := w.cores[coreID]; !exists {
		return errors.New(fmt.Sprintf("core %d not found", coreID))
	}

	sg := w.getSGroup(groupID)
	if sg == nil {
		return fmt.Errorf("SGroup %d not found on worker[%s]", groupID, w.name)
	}

	// Schedules |sg| on the new core.

	// Send gRPC to inform scheduler.
	if status, err := w.AttachChain(sg.tids, coreID); err != nil {
		return err
	} else if status.GetCode() != 0 {
		return errors.New(fmt.Sprintf("error from gRPC request AttachChain: %s", status.GetErrmsg()))
	}
	return nil
}

// Detaches a SGroup, indexed by |groupID|, on its running core.
// The SGroup is still pinned to its original running core, but won't
// get executed.
func (w *Worker) detachSGroup(groupID int) error {
	// Move it from core |coreId| to freeSGroups.
	//sGroup := w.cores[coreId].detachSGroup(groupID)

	sg := w.getSGroup(groupID)
	if sg == nil {
		return fmt.Errorf("SGroup %d not found on worker[%s]", groupID, w.name)
	}

	// Send gRPC to inform scheduler.
	if status, err := w.DetachChain(sg.tids, 0); err != nil {
		return err
	} else if status.GetCode() != 0 {
		return errors.New(fmt.Sprintf("error from gRPC request DetachChain: %s", status.GetErrmsg()))
	}
	return nil
}

// Unschedule all NF threads and shutdown CooperativeSched.
// Destroy all SGroups (Instances), FreeSGroups on worker |w|.
func (w *Worker) Close() error {
	errmsg := []string{}

	// Shutdowns and waits for all background go routines.
	w.op <- SHUTDOWN
	w.schedOp <- SHUTDOWN
	w.wg.Wait()

	// Sends gRPC to shutdown scheduler.
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

	if len(errmsg) > 0 {
		return errors.New(strings.Join(errmsg, ""))
	}

	fmt.Printf("worker[%s] is cleaned up\n", w.name)
	return nil
}
