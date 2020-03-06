
package controller

import (
)

// The controller of the FaaS system for NFV.
// |workers| are all the worker nodes (i.e. physical or virtual machines) in the system.
type FaaSController struct {
	workers map[string]*Worker
	//TODO: instances map[string]Instance
}

// Creates a new FaaS controller.
func NewFaaSController() *FaaSController{
	c := &FaaSController{
		workers: make(map[string]*Worker),
	}

	// Create worker for cluster nodes.
	// Now core 0 is reserved for scheduling executor.
	// TODO: Replace hard-code information with reading from k8s configurations.
	c.createWorker("ubuntu", "204.57.7.14", 10514, 10515, 1,8)
	c.createWorker("uscnsl", "204.57.7.3", 10514, 10515, 1,8)
	return c
}

func(c *FaaSController) createWorker(name string, ip string, vSwitchPort int, executorPort int,
	coreNumOffset int,coreCount int) {
	if _, exists := c.workers[name]; exists {
		return
	}
	c.workers[name] = newWorker(name, ip, vSwitchPort, executorPort, coreNumOffset, coreCount)
}

func (c *FaaSController) getWorker(nodeName string) *Worker {
	if _, exists := c.workers[nodeName]; !exists {
		return nil
	}
	return c.workers[nodeName]
}