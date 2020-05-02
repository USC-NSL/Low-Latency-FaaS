package controller

import (
	"fmt"
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
// It waits for control messages to create FreeSgroups. The goal
// is to maintain a pool of NIC queues for buffering packets.
func (w *Worker) CreateFreeSGroups(op chan FaaSOP) {
	var msg FaaSOP

	for {
		msg = <-op

		if msg == FREE_SGROUP {
			// TODO(Jianfeng): handle errors.
			go w.createFreeSGroup()
		}

		if msg == SHUTDOWN {
			break
		}
	}
}

// Go-routine function for creating a FreeSgroup.
// Creates and returns a free SGroup |sg|. |sg| initializes a NIC
// queue (at most 4K packets) which can be used by a NF chain later.
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

	for i, funcType := range dag.chains {
		isIngress := "false"
		isEgress := "false"
		if i == 0 {
			isIngress = "true"
		} else if i == len(dag.chains)-1 {
			isEgress = "true"
		}
		vPortIncIdx, vPortOutIdx := i, i+1

		ins, err := w.createInstance(funcType, pcieIdx, "false", isIngress, isEgress, vPortIncIdx, vPortOutIdx)
		if err != nil {
			glog.Errorf("Failed to create nf[%s]. %s\n", funcType, err)

			// Cleanup.. |sg| is moved to |w.freeSGroups|.
			w.destroySGroup(sg)
			return
		}

		sg.AppendInstance(ins)
	}

	// Adds |sg| to |w|'s active |sgroups|, and |dag|'s
	// active |sgroups|.
	w.sgMutex.Lock()
	w.sgroups = append(w.sgroups, sg)
	w.sgMutex.Unlock()

	dag.sgroups = append(dag.sgroups, sg)
}
