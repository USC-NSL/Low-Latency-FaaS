package controller

import (
	"errors"
	"fmt"
	"sort"

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
	var dag *DAG = nil
	for _, d := range c.dags {
		if d.Match(srcIP, dstIP, srcPort, dstPort, proto) {
			dag = d
		}
	}

	// The flow does not match any activated DAGs. Just ignore it.
	// TODO(Jianfeng): handle the drop case properly.
	if dag == nil || (!dag.isActive) {
		return "None", nil
	}

	var sg *SGroup = nil
	// Picks an active SGroup |sg| and assigns the flow to it.
	if sg = dag.findActiveSGroup(); sg != nil {
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
	return "None", errors.New(fmt.Sprintf("No enough resources"))
}

// Selects an active |SGroup| for the logical NF DAG |g|. Picks
// the one with the lowest traffic load (packet rate).
func (g *DAG) findActiveSGroup() *SGroup {
	var selected *SGroup = nil
	for _, sg := range g.sgroups {
		if selected == nil {
			selected = sg
		} else if selected.pktRateKpps < sg.pktRateKpps {
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
// It tries to pack SGroups into a minimum number of CPU cores.
// Algorithm: Best Fit Decreasing.
func (w *Worker) schedule() {
	// This function stops all updates on Worker |w|.
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	sort.Sort(sort.Reverse(w.sgroups))

	coreID := 1
	load := 0
	for _, sg := range w.sgroups {
		if !sg.IsReady() { continue }

		sgLoad := sg.GetLoad()

		if load + sgLoad < 80 {
			load = load + sgLoad
		} else if (coreID < 7) {
			coreID += 1
			load = sgLoad
		} else {
			glog.Errorf("Worker[%s] runs out of cores", w.name)
			break
		}

		// Sends a gRPC request to enforce the scheduling.
		err := sg.attachSGroup(coreID)
		if err != nil {
			glog.Errorf("Failed to attach SGroup[%d] to Core[%d]", sg.ID(), coreID)
		}
	}
}
