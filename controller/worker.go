package controller

import (
	"errors"
	"fmt"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	kubectl "github.com/USC-NSL/Low-Latency-FaaS/kubectl"
	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
	glog "github.com/golang/glog"
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
// |instanceWaitingPool| is a pool for instances that are on start-up.
type Worker struct {
	grpc.VSwitchGRPCHandler
	grpc.SchedulerGRPCHandler
	name                string
	ip                  string
	cores               map[int]*Core
	sgroups             map[int]*SGroup
	freeSGroups         []*SGroup
	instancePortPool    *utils.IndexPool
	pciePool            *utils.IndexPool
	instanceWaitingPool InstanceWaitingPool
	op                  chan FaaSOP
}

func newWorker(name string, ip string, vSwitchPort int, schedulerPort int, coreNumOffset int, coreNum int) *Worker {
	w := Worker{
		name:        name,
		ip:          ip,
		cores:       make(map[int]*Core),
		sgroups:     make(map[int]*SGroup),
		freeSGroups: make([]*SGroup, 0),
		// Ports taken by instances are between [50052, 51051]
		instancePortPool: utils.NewIndexPool(50052, 1000),
		pciePool:         utils.NewIndexPool(0, len(PCIeMappings)),
		op:               make(chan FaaSOP, 10),
	}

	// TODO(Zhuojin): remove VSwitchGRPCHandler.
	//if err := w.VSwitchGRPCHandler.EstablishConnection(fmt.Sprintf("%s:%d", ip, vSwitchPort)); err != nil {
	//	fmt.Println("Fail to connect with vSwitch: " + err.Error())
	//}

	/* TODO(Jianfeng): fix per-worker scheduling.
	if err := w.SchedulerGRPCHandler.EstablishConnection(fmt.Sprintf("%s:%d", ip, schedulerPort)); err != nil {
		fmt.Println("Fail to connect with scheduler: " + err.Error())
	}*/

	// coreId is ranged between [coreNumOffset, coreNumOffset + coreNum)
	for i := 0; i < coreNum; i++ {
		w.cores[i+coreNumOffset] = newCore()
	}

	// Starts a background routine for maintaining |freeSGroups|
	go w.CreateFreeSGroups(w.op)

	// Initializes |freeSGroups|
	w.maintainFreeSGroup()

	return &w
}

func (w *Worker) String() string {
	info := fmt.Sprintf("Worker [%s] at %s \n Core:", w.name, w.ip)
	for coreId, core := range w.cores {
		info += fmt.Sprintf("\n  %d %s", coreId, core)
	}
	info += "\n Free instances:"
	for _, sg := range w.freeSGroups {
		info += fmt.Sprintf("\n  %s", sg)
	}
	return info + "\n"
}

// Creates an NF instance with type |funcType|. Waits for |tid|
// sent from the instance.
func (w *Worker) createInstance(funcType string, pcieIdx int, isIngress string, isEgress string, vPortIncIdx int, vPortOutIdx int) *Instance {
	port := w.instancePortPool.GetNextAvailable()
	// By default, the instance will run on core 0.

	if _, err := kubectl.K8sHandler.CreateDeployment(w.name, funcType, port, PCIeMappings[pcieIdx], isIngress, isEgress, vPortIncIdx, vPortOutIdx); err != nil {
		msg := fmt.Sprintf("Fail to create a new instance. %s", err)
		glog.Error(msg)
		w.instancePortPool.Free(port)
		return nil
	}

	// Succeed
	instance := newInstance(funcType, w.ip, port)
	w.instanceWaitingPool.add(instance)
	instance.waitTid()
	return instance
}

// Destroys an NF |ins|. Note: this function only gets called
// when freeing a sGroup.
func (w *Worker) destroyInstance(ins *Instance) error {
	fmt.Println(w.name)
	err := kubectl.K8sHandler.DeleteDeployment(w.name, ins.funcType, ins.port)
	if err == nil {
		// Succeed
		w.instancePortPool.Free(ins.port)
	}
	return err
}

// Creates and returns a free SGroup that initializes a NIC queue.
// |sg| can be configured to run a NF chain later by creating NF
// containers. It initializes a NIC queue that can buffer packets
// shortly (at most 4K packets).
func (w *Worker) createFreeSGroup() (*SGroup, error) {
	fmt.Println("0")
	pcieIdx := w.pciePool.GetNextAvailable()
	fmt.Println("1")
	sg := newSGroup(w, pcieIdx)
	fmt.Println("2")
	if sg == nil {
		w.pciePool.Free(pcieIdx)
		return nil, errors.New("Failed to create a free SGroup")
	}

	w.freeSGroups = append(w.freeSGroups, sg)
	return sg, nil
}

// Destroys a SGroup |sg|.
// Note: Unable to destroy a SGroup which is currently attached to a core.
func (w *Worker) destroyFreeSGroup(sg *SGroup) error {
	if err := w.destroyInstance(sg.ingress); err != nil {
		msg := fmt.Sprintf("Worker[%s] failed to remove SGroup[%d]. %s", sg.worker, sg.groupId, err)
		return errors.New(msg)
	}

	w.pciePool.Free(sg.pcieIdx)
	return nil
}

// Returns a free sGroup |sg| in |w.freeSGroups|. |sg| is removed
// from |w|'s freeSGroups. Returns nil if |w.freeSGroups| is empty.
func (w *Worker) getFreeSGroup() *SGroup {
	n := len(w.freeSGroups)
	if n >= 1 {
		sg := w.freeSGroups[n-1]
		// Removes |sg| from w.freeSGroups.
		w.freeSGroups = w.freeSGroups[:(n - 1)]
		return sg
	}

	return nil
}

/*
// Find runnable |sGroup| on a core to place the logical chain with |funcTypes|.
// If not found, return nil.
func (w *Worker) findRunnableSGroup(funcTypes []string) *SGroup {
	for _, core := range w.cores {
		for _, sGroup := range core.sGroups {
			if sGroup.match(funcTypes) {
				return sGroup
			}
		}
	}
	return nil
}

// TODO: Adjust core selection strategy.
func (w *Worker) findAvailableCore() int {
	for coreId, core := range w.cores {
		if len(core.sGroups) == 0 {
			return coreId
		}
	}
	return -1
}
*/

// Updates |qlen| and |kpps| for the sgroup with |groupId|.
// TODO (Jianfeng): trigger extra scaling operations.
func (w *Worker) updateSGroup(groupId int, qlen int, kpps int) error {
	if _, exists := w.sgroups[groupId]; !exists {
		return errors.New(fmt.Sprintf("sgroup %d not found", groupId))
	}

	w.sgroups[groupId].incQueueLength = qlen
	w.sgroups[groupId].pktRateKpps = kpps
	return nil
}

// Move sGroup with |groupId| from freeSGroups to core |coreId|.
func (w *Worker) attachSGroup(groupId int, coreId int) error {
	if _, exists := w.cores[coreId]; !exists {
		return errors.New(fmt.Sprintf("core %d not found", coreId))
	}

	idx := -1
	for i, group := range w.freeSGroups {
		if group.groupId == groupId {
			idx = i
		}
	}
	if idx == -1 {
		return errors.New(fmt.Sprintf("cannot find free sGroup (id = %d) at node %s", groupId, w.name))
	}
	// Move it from freeSGroups to a core.
	sGroup := w.freeSGroups[idx]
	w.freeSGroups = append(w.freeSGroups[:idx], w.freeSGroups[idx+1:]...)
	w.cores[coreId].sGroups = append(w.cores[coreId].sGroups, sGroup)
	// Send gRPC to inform scheduler.
	if status, err := w.AttachChain(sGroup.tids, coreId); err != nil {
		return err
	} else if status.GetError() != 0 {
		return errors.New(fmt.Sprintf("error from gRPC request AttachChain: %s", status.GetMessage()))
	}
	return nil
}

// Migrate sGroup with |groupId| from core |coreFrom| to core |coreTo|.
func (w *Worker) migrateSGroup(groupId int, coreFrom int, coreTo int) error {
	if err := w.detachSGroup(groupId, coreFrom); err != nil {
		return err
	}
	if err := w.attachSGroup(groupId, coreTo); err != nil {
		return err
	}
	return nil
}

// Detach sGroup with |groupId| from core |coreId| and move it to freeSGroups.
func (w *Worker) detachSGroup(groupId int, coreId int) error {
	if _, exists := w.cores[coreId]; !exists {
		return errors.New(fmt.Sprintf("core %d not found", coreId))
	}

	// Move it from core |coreId| to freeSGroups.
	sGroup := w.cores[coreId].detachSGroup(groupId)
	if sGroup == nil {
		return errors.New(fmt.Sprintf("cannot find sGroup (id = %d) on core %d of node %s", groupId, coreId, w.name))
	}
	w.freeSGroups = append(w.freeSGroups, sGroup)
	// Send gRPC to inform scheduler.
	if status, err := w.DetachChain(sGroup.tids, coreId); err != nil {
		return err
	} else if status.GetError() != 0 {
		return errors.New(fmt.Sprintf("error from gRPC request DetachChain: %s", status.GetMessage()))
	}
	return nil
}

// Detach and destroy all sGroups on the worker.
func (w *Worker) cleanUp() error {
	/*
		// Detach all sGroups.
		for coreId, core := range w.cores {
			for len(core.sGroups) > 0 {
				sGroup := core.sGroups[0]
				if err := w.detachSGroup(sGroup.groupId, coreId); err != nil {
					return err
				}
			}
		}
	*/
	fmt.Println(len(w.freeSGroups))
	for len(w.freeSGroups) > 0 {
		idx := len(w.freeSGroups)
		sg := w.freeSGroups[idx-1]
		fmt.Println(idx)
		if err := w.destroyFreeSGroup(sg); err != nil {
			return err
		}

		//w.freeSGroups = append(w.freeSGroups[:i], w.freeSGroups[i+1:]...)
		w.freeSGroups = w.freeSGroups[:(idx - 1)]
	}

	return nil
}
