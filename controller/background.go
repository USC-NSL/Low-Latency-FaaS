package controller

import (
	"fmt"
	"strings"
	"sync"
	"time"

	kubectl "github.com/USC-NSL/Low-Latency-FaaS/kubectl"
	glog "github.com/golang/glog"
)

// This is the place to implement FaaSController go routines
// that run in the background.
// 1. Worker
// - Creates free SGroups in the background.
// - Deploys NF containers in the background.
// 2. Schedule
// - Optimizes CPU scheduler in the background.

type FaaSOP int

const (
	_                  = iota // Ignore first value.
	FREE_SGROUP FaaSOP = 1 << (10 * iota)
	SHUTDOWN
)

func (op FaaSOP) String() string {
	switch {
	case op == FREE_SGROUP:
		return fmt.Sprintf("CREATE_FREE_SGROUP")
	default:
		return fmt.Sprintf("%d", op)
	}
}

// Long-running Go-routine function at each worker.
// This function waits for control messages to create FreeSgroups.
// The goal is to create a pool of NIC queues concurrently.
func (w *Worker) RunFreeSGroupFactory(op chan FaaSOP) {
	var msg FaaSOP

	for {
		msg = <-op

		if msg == FREE_SGROUP {
			// TODO(Jianfeng): handle errors.
			go w.createFreeSGroup()
		}

		if msg == SHUTDOWN {
			w.wg.Done()
			return
		}
	}
}

// Go-routine function for creating a FreeSgroup.
// Creates and returns a free SGroup |sg|. |sg| initializes a NIC
// queue (at most 4K packets) which can be used by an NF chain later.
// Blocked until the pod is running.
func (w *Worker) createFreeSGroup() *SGroup {
	pcieIdx := w.pciePool.GetNextAvailable()
	sg := newSGroup(w, pcieIdx)
	if sg == nil {
		w.pciePool.Free(pcieIdx)
		return nil
	}

	start := time.Now()
	for time.Now().Unix()-start.Unix() < 20 {
		status := kubectl.K8sHandler.GetPodStatusByName(sg.manager.podName)
		if status == "Running" {
			// Succeeded.
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	w.sgMutex.Lock()
	w.freeSGroups = append(w.freeSGroups, sg)
	w.sgMutex.Unlock()
	return sg
}

// Go-routine function for deleting a FreeSGroup.
// Deletes a free SGroup |sg|. Blocked until the pod is deleted.
func (w *Worker) destroyFreeSGroup(sg *SGroup, wg *sync.WaitGroup) {
	defer wg.Done()

	if err := w.destroyInstance(sg.manager); err != nil {
		glog.Errorf("Worker[%s] failed to remove SGroup[%d]. %s", sg.worker, sg.ID(), err)
		return
	}

	start := time.Now()
	for time.Now().Unix()-start.Unix() < 20 {
		status := kubectl.K8sHandler.GetPodStatusByName(sg.manager.podName)
		if status == "NotExist" {
			// Deleted.
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	w.pciePool.Free(sg.pcieIdx)
}

// Scales up the NF |dag| by creating all related NF containers. Also
// sends a gRPC request to register all NF threads at the |w|'s
// CooperativeSched. |sg| is updated after this function finishes.
func (w *Worker) createSGroup(sg *SGroup, dag *DAG) {
	pcieIdx := sg.pcieIdx

	for i, nf := range dag.chains {
		funcType := []string{nf.funcType}
		cycleCost := nf.cycles
		isPrimary := false
		isIngress := false
		isEgress := false
		if i == 0 {
			isIngress = true
		}
		if i == len(dag.chains)-1 {
			isEgress = true
		}
		vPortIncIdx, vPortOutIdx := i, i+1

		ins, err := w.createInstance(funcType, cycleCost, pcieIdx, kFaaSStartCoreID, isPrimary, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
		if err != nil {
			glog.Errorf("Failed to create nf[%s]. %s\n", funcType, err)

			// Cleanup.. |sg| is moved to |w.freeSGroups|.
			w.destroySGroup(sg)
			return
		}

		// Set ins.sg = sg
		sg.AppendInstance(ins)
	}

	// Add |sg| to |w.sgroups|.
	w.sgMutex.Lock()
	w.sgroups = append(w.sgroups, sg)
	w.sgroupTarget += 1
	w.sgMutex.Unlock()

	// Add |sg| to |dag|'s active |sgroups|.
	dag.sgroups = append(dag.sgroups, sg)
	sg.dag = dag

	// Check whether the sg is ready to serve traffic.
	// If yes, notify the cooperative scheduler.
	sg.isComplete = true
	sg.preprocessBeforeReady()
}

func (w *Worker) metronCreateSGroup(sg *SGroup, dag *DAG) {
	if sg == nil || dag == nil {
		return
	}
	if !sg.IsCoreIDValid() {
		return
	}

	pcieIdx := sg.pcieIdx

	nfTypes := make([]string, 0)
	cycleCost := 0
	for _, nf := range dag.chains {
		nfTypes = append(nfTypes, nf.funcType)
		cycleCost += nf.cycles
	}

	isPrimary := false
	isIngress := true
	isEgress := true
	vPortIncIdx, vPortOutIdx := 0, 1

	ins, err := w.createInstance(nfTypes, cycleCost, pcieIdx, sg.coreID, isPrimary, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
	if err != nil {
		glog.Errorf("Failed to create nf[%s]. %s\n", strings.Join(nfTypes, ","), err)

		// Cleanup.. |sg| is moved to |w.freeSGroups|.
		w.destroySGroup(sg)
		return
	}

	// Set ins.sg = sg
	sg.AppendInstance(ins)

	// Add |sg| to |w.sgroups|.
	w.sgMutex.Lock()
	w.sgroups = append(w.sgroups, sg)
	w.sgMutex.Unlock()

	// Add |sg| to |dag|'s active |sgroups|.
	dag.sgroups = append(dag.sgroups, sg)
	sg.dag = dag

	// Check whether the sg is ready to serve traffic.
	// If yes, notify the cooperative scheduler.
	sg.isComplete = true
	sg.preprocessBeforeReady()
}
