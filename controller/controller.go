package controller

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

var runFaaSTest bool

// The controller of the FaaS system for NFV.
// |ToRGRPCHandler| are functions to handle gRPC requests to the ToR switch.
// |workers| are all the worker nodes (i.e. physical or virtual machines) in the system.
// |instances| maintains all running NF instances.
// |dags| maintains all logical representations of NF DAGs.
type FaaSController struct {
	grpc.ToRGRPCHandler
	workers map[string]*Worker
	dags    map[string]*DAG
}

// Creates a new FaaS controller.
func NewFaaSController(isTest bool) *FaaSController {
	c := &FaaSController{
		workers: make(map[string]*Worker),
		dags:    make(map[string]*DAG),
	}
	runFaaSTest = isTest

	// Initializes all worker nodes when starting a |FaaSController|.
	// Now core 0 is reserved for the scheduler on the machine.
	// TODO: Replace hard-code information with reading from k8s configurations.
	//c.createWorker("uscnsl", "204.57.7.2", 10514, 10515, 1, 7)
	c.createWorker("ubuntu", "204.57.7.11", 10514, 10515, 1, 7)
	return c
}

func (c *FaaSController) createWorker(name string, ip string, vSwitchPort int, schedulerPort int,
	coreNumOffset int, coreCount int) {
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

// Adds an NF |funcType| to a |user|'s DAG. If |user| does not
// exist, first creates a new |user| in |FaaSController|.
// |user| is a string that represents the user's ID.
func (c *FaaSController) AddNF(user string, funcType string) error {
	if _, exists := c.dags[user]; !exists {
		c.dags[user] = newDAG()
	}

	return c.dags[user].addNF(funcType)
}

// Connects two NFs |upNF| -> |downNF| to a |user|'s DAG.
// |user| is a string that represents the user's ID.
func (c *FaaSController) ConnectNFs(user string, upNF string, downNF string) error {
	if _, exists := c.dags[user]; !exists {
		return errors.New(fmt.Sprintf("User [%s] has no NFs.", user))
	}

	return c.dags[user].connectNFs(upNF, downNF)
}

// Starts running a NF DAG.
func (c *FaaSController) ActivateDAG(user string) {
	for user, dag := range c.dags {
		if user == user || user == "all" {
			dag.Activate()
		}
	}
}

// Prints all DAGs managed by |FaaSController|.
func (c *FaaSController) ShowNFDAGs(user string) {
	for u, dag := range c.dags {
		if user == u || user == "all" {
			fmt.Printf("[%s] deploys NF DAG [actived=%t]:\n", u, dag.isActive)

			// Prints the NF graphs to the terminal.
			drawCmd := exec.Command("graph-easy")
			drawIn, _ := drawCmd.StdinPipe()
			drawOut, _ := drawCmd.StdoutPipe()
			drawCmd.Start()
			drawIn.Write([]byte(dag.String()))
			drawIn.Close()
			drawBytes, _ := ioutil.ReadAll(drawOut)
			fmt.Println(string(drawBytes))
			drawCmd.Wait()
		}
	}
}

/*
// Note: Only used for test. Call function FindCoreToServeSGroup to create sGroup instead.
func (c *FaaSController) CreateSGroup(nodeName string, funcTypes []string) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}

	_, err := c.workers[nodeName].createSGroup(funcTypes)
	return err
}

func (c *FaaSController) DestroySGroup(nodeName string, groupId int) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}
	return c.workers[nodeName].destroyFreeSGroup(groupId)
}
*/

func (c *FaaSController) AttachSGroup(nodeName string, groupId int, coreId int) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}
	return c.workers[nodeName].attachSGroup(groupId, coreId)
}

func (c *FaaSController) DetachSGroup(nodeName string, groupId int, coreId int) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}
	return c.workers[nodeName].detachSGroup(groupId, coreId)
}

// Detach and destroy all sGroups.
func (c *FaaSController) CleanUpAllWorkers() error {
	for _, w := range c.workers {
		if err := w.cleanUp(); err != nil {
			return err
		}
	}
	return nil
}

// Called when receiving gRPC request for an new instance setting up.
// The new instance is on worker |nodeName| with allocated port |port| and TID |tid|.
func (c *FaaSController) InstanceSetUp(nodeName string, port int, tid int) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}
	c.workers[nodeName].insStartupPool.remove(port, tid)
	return nil
}

// Called when receiving gRPC request updating traffic info.
// |qlen| is the NIC rx queue length. |kpps| is the traffic volume.
// Returns error if this controller failed to update traffic info.
func (c *FaaSController) InstanceUpdateStats(nodeName string, groupId int, qlen int, kpps int) error {
	if _, exists := c.workers[nodeName]; !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}

	return c.workers[nodeName].updateSGroup(groupId, qlen, kpps)
}
