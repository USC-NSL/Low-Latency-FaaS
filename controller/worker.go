package controller

import (
	"errors"
	"fmt"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	kubectl "github.com/USC-NSL/Low-Latency-FaaS/kubectl"
	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
)

// The abstraction of worker nodes.
// |VSwitchGRPCHandler| and |SchedulerGRPCHandler| are functions
// to handle gRPC requests to vSwitch and scheduler on the node.
// |name| is the name of the node in kubernetes.
// |ip| is the ip address of the worker node.
// |vSwitchPort| is BESS gRPC port on host (e.g. FlowGen).
// |schedulerPort| is Cooperativesched gRCP port on host.
// |cores| is the abstraction of cores on the node (a mapping from real core number to the core).
// |freeInstances| are NF instances not pinned to any core yet (but in memory).
// |instancePortPool| manages ports taken by instances on the node.
// This is to prevent conflicts on host TCP ports.
type Worker struct {
	grpc.VSwitchGRPCHandler
	grpc.SchedulerGRPCHandler
	name             string
	ip               string
	vSwitchPort      int
	schedulerPort    int
	cores            map[int]*Core
	freeSGroups      []*SGroup
	instancePortPool *utils.IndexPool
}

func newWorker(name string, ip string, vSwitchPort int, schedulerPort, coreNumOffset int, coreNum int) *Worker {
	worker := Worker{
		name:          name,
		ip:            ip,
		vSwitchPort:   vSwitchPort,
		schedulerPort: schedulerPort,
		cores:         make(map[int]*Core),
		freeSGroups:   make([]*SGroup, 0),
		// Ports taken by instances are between [50052, 51051]
		instancePortPool: utils.NewIndexPool(50052, 1000),
	}

	// coreId is ranged between [coreNumOffset, coreNumOffset + coreNum)
	for i := 0; i < coreNum; i++ {
		worker.cores[i+coreNumOffset] = newCore()
	}
	return &worker
}

func (w *Worker) String() string {
	info := fmt.Sprintf("Worker [%s] at %s \n Core:", w.name, w.ip)
	for coreId, core := range w.cores {
		info += fmt.Sprintf("\n  %d %s", coreId, core)
	}
	info += "\n Free instances:"
	for _, instance := range w.freeSGroups {
		info += fmt.Sprintf("\n  %s", instance)
	}
	return info + "\n"
}

// Create an NF instance with type |funcType|. Waiting until receiving the tid from the instance.
// Note: Call it only when creating a sGroups.
func (w *Worker) createInstance(funcType string) (*Instance, error) {
	port := w.instancePortPool.GetNextAvailable()
	// By default, the instance will run on core 0.

	if _, err := kubectl.K8sHandler.CreateDeployment(w.name, 0, funcType, port); err != nil {
		// Fail to create a new instance.
		w.instancePortPool.Free(port)
		return nil, err
	}

	// Succeed
	instance := newInstance(funcType, w.ip, port)
	instance.waitTid()
	return instance, nil
}

// Search and destroy an NF |instance|.
// Note: Call it only when freeing a sGroups.
func (w *Worker) destroyInstance(instance *Instance) error {
	err := kubectl.K8sHandler.DeleteDeployment(w.name, instance.funcType, instance.port)
	if err == nil {
		// Succeed
		w.instancePortPool.Free(instance.port)
	}
	return err
}

// Create a |sGroup| by a chain of NFs |funcTypes| and store it in freeSGroups.
// To avoid busy waiting, call this function in go routine.
// Also send gRPC request to inform the scheduler.
func (w *Worker) createSGroup(funcTypes []string) (*SGroup, error) {
	instances := make([]*Instance, 0)
	for _, funcType := range funcTypes {
		newInstance, err := w.createInstance(funcType)
		if err != nil {
			// Fail to create a new instance.
			for _, instance := range instances {
				w.destroyInstance(instance)
			}
			return nil, err
		}
		instances = append(instances, newInstance)
	}
	// Send gRPC request to inform the scheduler.
	for _, instance := range instances {
		w.SetUpThread(instance.tid)
	}
	sGroup := newSGroup(w, instances)
	w.freeSGroups = append(w.freeSGroups, sGroup)
	return sGroup, nil
}

// Search and destroy a free sGroup by its groupId (equal to the tid of its first NF instance).
// Also free all instances within it.
func (w *Worker) destroySGroup(groupId int) error {
	for i, sGroup := range w.freeSGroups {
		if sGroup.groupId == groupId {
			for _, instance := range sGroup.instances {
				w.destroyInstance(instance)
			}
			w.freeSGroups = append(w.freeSGroups[:i], w.freeSGroups[i+1:]...)
			return nil
		}
	}
	return errors.New(fmt.Sprintf("could not find sGroup (id = %d) in %s", groupId, w.name))
}

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

// Find free |sGroup| in freeSGroups to place the logical chain with |funcTypes|.
// If not found, return nil.
func (w *Worker) findFreeSGroup(funcTypes []string) *SGroup {
	for _, sGroup := range w.freeSGroups {
		if sGroup.match(funcTypes) {
			return sGroup
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

// Attach a new |sGroup| from freeSGroups on core |coreId|.
func (w *Worker) attachSGroup(sGroup *SGroup, coreId int) error {
	idx := -1
	for i, group := range w.freeSGroups {
		if group == sGroup {
			idx = i
		}
	}
	if idx == -1 {
		return errors.New(fmt.Sprintf("cannot find free sGroup (id = %d) at node %s", sGroup.groupId, w.name))
	}
	// Move it from freeSGroups to a core.
	w.freeSGroups = append(w.freeSGroups[:idx], w.freeSGroups[idx+1:]...)
	w.cores[coreId].sGroups = append(w.cores[coreId].sGroups, sGroup)
	// Send gRPC to inform scheduler.
	w.AttachChain(sGroup.tids, coreId)
	return nil
}

func (w *Worker) migrateSGroup() {

}
