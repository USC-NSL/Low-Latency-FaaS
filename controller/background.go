package controller

import (
	"fmt"

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
			w.createFreeSGroup()
		}

		if msg == SHUTDOWN {
			break
		}
	}
}

// Go-routine function when setting up an NF DAG.
// Scales up the NF |dag| by creating all related NF containers. Also
// sends a gRPC request to register all NF threads at the |w|'s
// CooperativeSched. |sg| is updated after this function finishes.
func (w *Worker) createSGroup(sg *SGroup, dag *DAG) {
	pcieIdx := sg.pcieIdx

	for i, funcType := range dag.chains {
		isIngress := "false"
		isEgress := "false"
		if i == len(dag.chains)-1 {
			isEgress = "true"
		}
		vPortIncIdx, vPortOutIdx := i, i+1

		ins := w.createInstance(funcType, pcieIdx, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
		if ins == nil {
			glog.Errorf("Failed to create nf[%s]\n", funcType)
			// Cleanup..
			for _, instance := range sg.instances {
				w.destroyInstance(instance)
			}
			w.freeSGroups = append(w.freeSGroups, sg)
			return
		}
		sg.instances = append(sg.instances, ins)
	}

	// Sends gRPC request to inform the scheduler.
	for _, instance := range sg.instances {
		w.SetUpThread(instance.tid)
	}

	// Adds |sg| to |w|'s active |sgroups|, and |dag|'s
	// active |sgroups|.
	w.sgroups[sg.groupId] = sg
	dag.sgroups = append(dag.sgroups, sg)
}
