
package controller

import (
	"errors"
	"fmt"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

// The controller of the FaaS system for NFV.
// |ToRGRPCHandler| are functions to handle gRPC requests to the ToR switch.
// |workers| are all the worker nodes (i.e. physical or virtual machines) in the system.
type FaaSController struct {
	grpc.ToRGRPCHandler
	workers map[string]*Worker
	//TODO: instances map[string]Instance
}

// Creates a new FaaS controller.
func NewFaaSController() *FaaSController {
	c := &FaaSController{
		workers: make(map[string]*Worker),
	}

	// Create worker for cluster nodes.
	// Now core 0 is reserved for the scheduler on the machine.
	// TODO: Replace hard-code information with reading from k8s configurations.
	c.createWorker("uscnsl", "204.57.7.3", 10514, 10515, 1,7)
	c.createWorker("ubuntu", "204.57.7.14", 10514, 10515, 1,7)
	return c
}

func(c *FaaSController) createWorker(name string, ip string, vSwitchPort int, schedulerPort int,
	coreNumOffset int,coreCount int) {
	if _, exists := c.workers[name]; exists {
		return
	}
	c.workers[name] = newWorker(name, ip, vSwitchPort, schedulerPort, coreNumOffset, coreCount)
}

func (c *FaaSController) getWorker(nodeName string) *Worker {
	if _, exists := c.workers[nodeName]; !exists {
		return nil
	}
	return c.workers[nodeName]
}

func (c *FaaSController) GetWorkersInfo() string {
	info := ""
	for _, worker := range c.workers {
		info += worker.String()
	}
	return info
}

func (c *FaaSController) GetWorkerInfoByName(nodeName string) string {
	if worker, exists := c.workers[nodeName]; exists {
		return worker.String()
	}
	return ""
}

func (c *FaaSController) CreateInstance(nodeName string, funcType string) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}

	// TODO: check funcType here.
	return c.workers[nodeName].createInstance(funcType)
}

func (c *FaaSController) DestroyInstance(nodeName string, funcType string, port int) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}

	return c.workers[nodeName].destroyInstance(funcType, port)
}

func (c *FaaSController) CleanUpWorker(nodeName string) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}
	worker := c.workers[nodeName]
	for len(worker.freeInstances) > 0 {
		instance := worker.freeInstances[0]
		if err := worker.destroyInstance(instance.funcType, instance.port); err != nil {
			return err
		}
	}
	return nil
}

func (c *FaaSController) CleanUpAllWorkers() error {
	for _, worker := range c.workers {
		if err := c.CleanUpWorker(worker.name); err != nil {
			return err
		}
	}
	return nil
}
