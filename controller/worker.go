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
// |cores| is the abstraction of cores on the node.
// |coreNumOffset| is for mapping from cores array to real physical core number.
// |coreNumOffset| + index (in cores array) = real core number.
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
	cores            []*Core
	coreNumOffset    int
	freeInstances    []*Instance
	instancePortPool *utils.IndexPool
}

func newWorker(name string, ip string, vSwitchport int, schedulerPort, coreNumOffset int, coreNum int) *Worker {
	worker := Worker{
		name:          name,
		ip:            ip,
		vSwitchPort:   vSwitchport,
		schedulerPort: schedulerPort,
		cores:         make([]*Core, coreNum),
		coreNumOffset: coreNumOffset,
		freeInstances: make([]*Instance, 0),
		// Ports taken by instances are between [50052, 51051]
		instancePortPool: utils.NewIndexPool(50052, 1000),
	}

	for i := 0; i < coreNum; i++ {
		worker.cores[i] = newCore()
	}
	return &worker
}

func (w *Worker) String() string {
	info := fmt.Sprintf("Worker [%s] at %s \n Core:", w.name, w.ip)
	for idx, core := range w.cores {
		info += fmt.Sprintf("\n  %d %s", idx+w.coreNumOffset, core)
	}
	info += "\n Free instances:"
	for _, instance := range w.freeInstances {
		info += fmt.Sprintf("\n  %s", instance)
	}
	return info + "\n"
}

// Create an NF instance with type |funcType|. By default, the instance will run on core 0.
func (w *Worker) createInstance(funcType string) error {
	port := w.instancePortPool.GetNextAvailable()
	// By default, the instance will run on core 0.
	_, err := kubectl.K8sHandler.CreateDeployment(w.name, 0, funcType, port)
	if err != nil {
		// Fail to create a new instance.
		w.instancePortPool.Free(port)
	} else {
		// Success
		instance := newInstance(funcType, w.ip, port)
		w.freeInstances = append(w.freeInstances, instance)
	}
	return err
}

// Search and destroy an NF instance with type |funcType| and port |hostPort|.
// Note: The port is a kind of unique id for each instance on the node).
func (w *Worker) destroyInstance(funcType string, hostPort int) error {
	for i, instance := range w.freeInstances {
		if instance.port == hostPort {
			err := kubectl.K8sHandler.DeleteDeployment(w.name, funcType, hostPort)
			if err == nil {
				// Success
				w.instancePortPool.Free(hostPort)
				w.freeInstances = append(w.freeInstances[:i], w.freeInstances[i+1:]...)
			}
			return err
		}
	}

	return errors.New(fmt.Sprintf("could not find %s(%d) in %s", funcType, hostPort, w.name))
}

// Find existing instance of NF |funcType|.
// If not found, return -1.
func (w *Worker) findAvailableInstance(funcType string) int {
	for idx, instance := range w.freeInstances {
		if instance.funcType == funcType {
			return idx
		}
	}
	return -1
}

// Scheduling a new |sGroup| on core |coreId|.
func (w *Worker) scheduleSGroup(sGroup []string, coreId int) error {
	instances := make([]*Instance, 0)
	for _, funcType := range sGroup {
		idx := w.findAvailableInstance(funcType)
		if idx == -1 {
			for _, instance := range instances {
				w.freeInstances = append(w.freeInstances, instance)
			}
			return errors.New(fmt.Sprintf("cannot find available instance for %s at node %s", funcType, w.name))
		}
		instances = append(instances, w.freeInstances[idx])
		w.freeInstances = append(w.freeInstances[:idx], w.freeInstances[idx+1:]...)
	}

	w.cores[coreId].sGroups = append(w.cores[coreId].sGroups, newSGroup(w, coreId, instances))
	// TODO: Send gRPC to inform instances and scheduler.
	return nil
}
