package controller

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	glog "github.com/golang/glog"
)

// The controller of the FaaS system for NFV.
// |ToRGRPCHandler| are functions to handle gRPC requests to the ToR switch.
// |workers| are all the worker nodes (i.e. physical or virtual machines) in the system.
// |instances| maintains all running NF instances.
// |dags| maintains all logical representations of NF DAGs.
type FaaSController struct {
	grpc.ToRGRPCHandler
	workers  map[string]*Worker
	dags     map[string]*DAG
	masterIP string
}

// Creates a new FaaS controller.
func NewFaaSController(isTest bool, cluster *Cluster) *FaaSController {
	c := &FaaSController{
		workers:  make(map[string]*Worker),
		dags:     make(map[string]*DAG),
		masterIP: cluster.Master.IP,
	}

	// Initializes all worker nodes when starting a |FaaSController|.
	// Note: at each worker machine, core 0 is reserved for the scheduler on
	// the machine. Then, coreNum is set to Cores - 1 because these cores are
	// for running NFs.
	for i := 0; i < len(cluster.Workers); i++ {
		name := cluster.Workers[i].Name
		ip := cluster.Workers[i].IP
		coreNum := cluster.Workers[i].Cores - 1
		pcie := cluster.Workers[i].PCIe
		switchPort := uint32(cluster.Workers[i].SwitchPort)
		c.createWorker(name, ip, 1, coreNum, pcie, switchPort)
	}

	// If we are running tests, skip initializing all free SGroups because tests
	// are expected to create their free SGroups.
	if !isTest {
		// Initializes all hugepages and NIC queues.
		for _, w := range c.workers {
			w.createAllFreeSGroups()
			w.createSched()
		}
	}

	return c
}

func (c *FaaSController) createWorker(name string, ip string,
	coreNumOffset int, coreCount int, pcie []string, switchPort uint32) {
	if _, exists := c.workers[name]; exists {
		return
	}

	c.workers[name] = NewWorker(name, ip, coreNumOffset, coreCount, pcie, switchPort)
}

func (c *FaaSController) getWorker(nodeName string) *Worker {
	if _, exists := c.workers[nodeName]; !exists {
		return nil
	}
	return c.workers[nodeName]
}

// This function cleans up the FaaSController |c|.
// Cleans up all associated FaaS worker nodes.
func (c *FaaSController) Close() error {
	errmsg := []string{}

	for _, w := range c.workers {
		if err := w.Close(); err != nil {
			errmsg = append(errmsg, fmt.Sprintf("worker[%s] didn't close. Reason: %v\n", w.name, err))
		}
	}

	if len(errmsg) > 0 {
		return errors.New(strings.Join(errmsg, ""))
	}

	// Succeed.
	return nil
}

// Note: CLI-only functions.
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

// Adds an NF of |funcType| to a |user|'s DAG. Returns an integral
// handler of this NF. |user| represents the user's ID. If |user|
// does not exist, creates a new |user| in |FaaSController|.
func (c *FaaSController) AddNF(user string, funcType string) int {
	if _, exists := c.dags[user]; !exists {
		c.dags[user] = newDAG()
	}

	return c.dags[user].addNF(funcType)
}

func (c *FaaSController) AddDummyNF(user string, funcType string) int {
	if _, exists := c.dags[user]; !exists {
		c.dags[user] = newDAG()
	}

	return c.dags[user].addDummyNF(funcType)
}

// Connects two NFs |upNF| -> |downNF| to a |user|'s DAG.
// |user| is a string that represents the user's ID.
func (c *FaaSController) ConnectNFs(user string, upNF int, downNF int) error {
	if _, exists := c.dags[user]; !exists {
		return errors.New(fmt.Sprintf("User [%s] has no NFs.", user))
	}

	return c.dags[user].connectNFs(upNF, downNF)
}

// Starts running a NF DAG.
func (c *FaaSController) ActivateDAG(user string) error {
	dag, exists := c.dags[user]
	if !exists {
		return errors.New(fmt.Sprintf("User [%s] has no NFs.", user))
	}

	// For testing only, incoming packets always have a dstPort 8080.
	dag.addFlow("", "", 0, 8080, 0)

	dag.Activate()

	for {
		sg := c.getFreeSGroup()
		if sg == nil {
			break
		}

		sg.worker.createSGroup(sg, dag)
	}

	glog.Info("DAG is activated.")
	return nil
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

func (c *FaaSController) CreateSGroup(nodeName string, nfs []string) error {
	w, exists := c.workers[nodeName]
	if !exists {
		return fmt.Errorf("Worker %s does not exist", nodeName)
	}

	dag := newDAG()
	for _, nf := range nfs {
		dag.addNF(nf)
	}
	if err := dag.Activate(); err != nil {
		return err
	}

	glog.Infof("Deploy a DAG %v", dag.chains)

	n := len(w.freeSGroups)
	if n <= 0 {
		return fmt.Errorf("Worker %s does not have free SGroups.", w.name)
	}
	var sg *SGroup = w.freeSGroups[n-1]
	w.freeSGroups = w.freeSGroups[:(n - 1)]

	w.createSGroup(sg, dag)
	return nil
}

func (c *FaaSController) DestroySGroup(nodeName string, groupID int) error {
	w, exists := c.workers[nodeName]
	if !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}

	sg := w.getSGroup(groupID)
	if sg == nil {
		return fmt.Errorf("SGroup %d not found by worker[%s]", groupID, w.name)
	}

	return c.workers[nodeName].destroySGroup(sg)
}

func (c *FaaSController) AttachSGroup(nodeName string, groupID int, coreId int) error {
	w, exists := c.workers[nodeName]
	if !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}

	sg := w.getSGroup(groupID)
	return sg.attachSGroup(coreId)
}

func (c *FaaSController) DetachSGroup(nodeName string, groupID int) error {
	w, exists := c.workers[nodeName]
	if !exists {
		return errors.New(fmt.Sprintf("worker %s not found", nodeName))
	}

	sg := w.getSGroup(groupID)
	return sg.detachSGroup()
}

// Note: gRPC functions

// Called when receiving gRPC request for an new instance setting up.
// The new instance is on worker |nodeName| with allocated port |port| and TID |tid|.
func (c *FaaSController) InstanceSetUp(nodeName string, port int, tid int) error {
	w, exists := c.workers[nodeName]
	if !exists {
		return fmt.Errorf("Worker[%s] does not exist", nodeName)
	}

	ins := w.insStartupPool.get(port)
	if ins == nil || ins.sg == nil {
		return errors.New(fmt.Sprintf("SGroup not found"))
	}

	ins.sg.UpdateTID(port, tid)
	return nil
}

// Called when receiving gRPC request updating traffic info.
// |qlen| is the NIC rx queue length. |kpps| is the incoming traffic
// rate in (Kpps). |cycle| is the per-packet cycle cost.
// Returns error if this controller failed to update traffic info.
func (c *FaaSController) InstanceUpdateStats(nodeName string, port int, qlen int, kpps int, cycle int) error {
	w, exists := c.workers[nodeName]
	if !exists {
		return fmt.Errorf("Worker[%s] does not exist", nodeName)
	}

	ins := w.insStartupPool.get(port)
	if ins == nil || ins.sg == nil {
		return fmt.Errorf("SGroup not found")
	}

	ins.UpdateTrafficInfo(qlen, kpps, cycle)
	ins.sg.UpdateTrafficInfo()
	return nil
}

// Set runtime cycles for instance on worker |nodeName| with port |port|.
func (c *FaaSController) SetCycles(nodeName string, port int, cyclesPerPacket int) error {
	w, exists := c.workers[nodeName]
	if !exists {
		return fmt.Errorf("Worker[%s] does not exist", nodeName)
	}
	return w.setCycles(port, cyclesPerPacket)
}

// Set batch size and batch number for instance on worker |nodeName| with port |port|.
// See message.proto for more information.
func (c *FaaSController) SetBatch(nodeName string, port int, batchSize int, batchNumber int) (string, error) {
	w, exists := c.workers[nodeName]
	if !exists {
		return "", fmt.Errorf("Worker[%s] does not exist", nodeName)
	}
	return w.setBatch(port, batchSize, batchNumber)
}
