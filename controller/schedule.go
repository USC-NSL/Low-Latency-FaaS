package controller

import (
	"errors"
	"fmt"
	"sort"
	"time"

	glog "github.com/golang/glog"
)

// This is the place to implement resource allocation.
// First, when a flow arrives, |FaaSController| has to assign
// a sgroup to serve this flow;
// Second, NF chains are packed periodically based on real-time
// monitoring;

// Called when a new flow arrives at the ToR switch. FaaSController
// decides the target logical NF chain, and picks an active NF
// chain to serve this flow. Returns the selected NF chain's unique
// NIC MAC address.
// TODO: Complete the reasons for returning errors.
func (c *FaaSController) UpdateFlow(srcIP string, dstIP string,
	srcPort uint32, dstPort uint32, proto uint32) (string, error) {
	//return "00:00:00:00:00:01", nil
	var dag *DAG = nil
	for _, d := range c.dags {
		if d.Match(srcIP, dstIP, srcPort, dstPort, proto) {
			dag = d
			break
		}
	}

	// The flow does not match any activated DAGs. Just ignore it.
	if dag == nil || (!dag.isActive) {
		glog.Infof("This new flow does not match any DAG.")
		return "none", nil
	}

	sg := dag.findAvailableSGroupHighLoadFirst()
	// Picks an active SGroup |sg| and assigns the flow to it.
	if sg != nil {
		//glog.Infof("SGroup[%d], mac=%s, load=%d", sg.ID(), dmacMappings[sg.pcieIdx], sg.GetPktLoad())
		return dmacMappings[sg.pcieIdx], nil
	}

	// No active SGroups. Triggers a scale-up event.
	// 1. Finds a free SGroup |sg| (NIC queue resource);
	// 2. Starts to deploy a new DAG with |sg|;
	// 3. (Optional) Triggers background threads to prepare more SGroups.
	// 4. Assigns the flow to the selected NIC queue. Even if packets
	// get queued up at the NIC queue for a while.
	if sg = c.getFreeSGroup(); sg != nil {
		go sg.worker.createSGroup(sg, dag)
		return dmacMappings[sg.pcieIdx], nil
	}

	// All active SGroups are running heavily. No free SGroups
	// are available. Just drop the packet. (Ideally, we should
	// never reach here if the cluster has enough resources.)
	return "none", errors.New(fmt.Sprintf("No enough resources"))
}

// Selects an active |SGroup| for the logical NF DAG |g|. Picks
// the one with the lowest traffic load (packet rate).
func (g *DAG) findAvailableSGroup() *SGroup {
	var selected *SGroup = nil
	for _, sg := range g.sgroups {
		// Skips if there are instances not ready.
		if !sg.IsReady() {
			continue
		}

		// Skips overloaded SGroups.
		if sg.GetQLoad() > 40 && sg.GetPktLoad() > 60 {
			continue
		}

		if selected == nil {
			selected = sg
		} else if selected.GetPktRate() > sg.GetPktRate() {
			selected = sg
		}
	}
	if selected != nil {
		return selected
	}

	for _, sg := range g.sgroups {
		// Skips if there are instances not ready.
		if !sg.IsReady() {
			continue
		}

		// Skips overloaded SGroups.
		if sg.GetPktLoad() > 80 {
			continue
		}

		if selected == nil {
			selected = sg
		} else if selected.GetPktRate() > sg.GetPktRate() {
			selected = sg
		}
	}

	return selected
}

// Finds and returns a free |sGroup| in the cluster. Returns nil
// if no free sgroup is found.
func (c *FaaSController) getFreeSGroup() *SGroup {
	for _, worker := range c.workers {
		if sg := worker.getFreeSGroup(); sg != nil {
			return sg
		}
	}
	return nil
}

// The per-worker NF thread scheduler.
// This function monitors traffic loads for all deployed SGroups.
// It packs SGroups into a minimum number of CPU cores.
// Algorithm: Best Fit Decreasing.
func (w *Worker) scheduleOnce() {
	// Stops all updates on Worker |w| temporally.
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	sort.Sort(sort.Reverse(w.sgroups))

	coreID := 0
	load := 80

	for _, sg := range w.sgroups {
		if !sg.IsReady() {
			// Skips if |sg| is not ready for scheduling.
			continue
		} else if !sg.IsActive() {
			// Detaches |sg| if it is still being scheduled.
			if sg.IsSched() {
				if err := sg.detachSGroup(); err != nil {
					glog.Errorf("Failed to detach SGroup[%d]. %v", sg.ID(), err)
					continue
				}

				// |sg| should be detached successfully.
				if sg.IsSched() {
					glog.Errorf("SGroup[%d] was Detached but still running!", sg.ID())
				}
			}

			continue
		}

		sgLoad := sg.GetPktLoad()

		if load+sgLoad < 80 {
			load = load + sgLoad
		} else if coreID < 7 {
			coreID += 1
			load = sgLoad
		} else {
			glog.Errorf("Worker[%s] runs out of cores", w.name)
			break
		}

		// Enforce scheduling.
		// Note: be careful about deadlocks.
		if err := sg.attachSGroup(coreID); err != nil {
			glog.Errorf("Failed to attach SGroup[%d] to Core[%d]", sg.ID(), coreID)
		}
	}
}

// Selects an active |SGroup| for the logical NF DAG |g|. Picks
// the one with the highest CPU load (packet rate).
func (g *DAG) findAvailableSGroupHighLoadFirst() *SGroup {
	var selected *SGroup = nil
	for _, sg := range g.sgroups {
		// Skips if there are instances not ready.
		if !sg.IsReady() {
			continue
		}

		// Skips overloaded SGroups.
		if sg.GetQLoad() > 40 && sg.GetPktLoad() > 80 {
			continue
		}

		if selected == nil {
			selected = sg
		} else if selected.GetPktRate() < sg.GetPktRate() {
			selected = sg
		}
	}
	if selected != nil {
		return selected
	}

	for _, sg := range g.sgroups {
		// Skips if there are instances not ready.
		if !sg.IsReady() {
			continue
		}

		// Skips overloaded SGroups.
		if sg.GetPktLoad() > 80 {
			continue
		}

		if selected == nil {
			selected = sg
		} else if selected.GetPktRate() < sg.GetPktRate() {
			selected = sg
		}
	}

	return selected
}

// Algorithm:
// Best-Fit Decreasing for non-overloaded SGroups.
// Rescheduling for overloaded SGroups.
func (w *Worker) advancedFitScheduleOnce() {
	// Stops all updates on Worker |w| temporally.
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	sort.Sort(sort.Reverse(w.sgroups))

	coreID := 0
	load := 80

	for _, sg := range w.sgroups {
		if !sg.IsReady() {
			// Skips if |sg| is not ready for scheduling.
			continue
		} else if !sg.IsActive() {
			// Detaches |sg| if it is still being scheduled.
			if sg.IsSched() {
				if sg.GetCoreID() != 1 {
					if err := sg.attachSGroup(1); err != nil {
						glog.Errorf("Failed to attach SGroup[%d] to Core #1. %v", sg.ID(), err)
						continue
					}

					// |sg| should be attached successfully.
					if sg.GetCoreID() != 1 || !sg.IsSched() {
						glog.Errorf("SGroup[%d] was Attached to Core #1 but not running on it!", sg.ID())
					}
				}

				if err := sg.detachSGroup(); err != nil {
					glog.Errorf("Failed to detach SGroup[%d]. %v", sg.ID(), err)
					continue
				}

				// |sg| should be detached successfully.
				if sg.IsSched() {
					glog.Errorf("SGroup[%d] was Detached but still running!", sg.ID())
				}
			}

			continue
		}

		sgLoad := sg.GetPktLoad()

		if load+sgLoad < 80 {
			load = load + sgLoad
		} else if coreID < 7 {
			coreID += 1
			load = sgLoad
		} else {
			glog.Errorf("Worker[%s] runs out of cores", w.name)
			break
		}

		// Enforce scheduling.
		// Note: be careful about deadlocks.
		if err := sg.attachSGroup(coreID); err != nil {
			glog.Errorf("Failed to attach SGroup[%d] to Core[%d]", sg.ID(), coreID)
		}
	}
}

// Go routine that runs on each worker to rebalance traffic loads
// among available CPU cores.
func (w *Worker) ScheduleLoop() {
	for {
		select {
		case <-w.schedOp:
			w.wg.Done()
			return
		default:
			w.advancedFitScheduleOnce()
			time.Sleep(500 * time.Millisecond)
		}
	}
}
