package controller

import (
	"errors"
	"fmt"

	glog "github.com/golang/glog"
)

// This is the place to implement load balancing.
// FaaS Controller makes load balancing decisions on a per-flow basis.
// When a new flow arrives, the flow hits the ingress switch and then is
// reported to FaaS Controller. Then, FaaS Controller selects a sgroup to
// serve this flow.

// Called when a new flow arrives at the ToR switch. FaaSController
// decides the target logical NF chain, and picks an active NF
// chain to serve this flow. Returns the selected NF chain's unique
// NIC MAC address.
// TODO: Complete the reasons for returning errors.
func (c *FaaSController) UpdateFlow(srcIP string, dstIP string,
	srcPort uint32, dstPort uint32, proto uint32) (uint32, string, error) {
	//return 0, "00:00:00:00:00:01", nil

	if dstPort < 2000 {
		// Serve background traffic.
		sg := c.findAvailableSGroupOnIdleWorker(dstPort)
		if sg != nil {
			if !sg.IsActive() {
				sg.SetActive()
			}
			glog.Infof("Background traffic to %s via port %d", sg.worker.name, sg.worker.switchPort)
			return sg.worker.switchPort, DefaultDstMACs[sg.pcieIdx], nil
		}
	}

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
		return 0, "none", errors.New(fmt.Sprintf("unknown flowlet"))
	}

	sg := dag.findAvailableSGroupHighLoadFirst()
	// Picks an active SGroup |sg| and assigns the flow to it.
	if sg != nil {
		//glog.Infof("SGroup[%d], mac=%s, load=%d", sg.ID(), DefaultDstMACs[sg.pcieIdx], sg.GetPktLoad())
		// Turns |sg| to be active if sg is not active now.
		if !sg.IsActive() {
			sg.SetActive()
		}
		return sg.worker.switchPort, DefaultDstMACs[sg.pcieIdx], nil
	}

	// No active SGroups. Triggers a scale-up event.
	// 1. Finds a free SGroup |sg| (NIC queue resource);
	// 2. Starts to deploy a new DAG with |sg|;
	// 3. (Optional) Triggers background threads to prepare more SGroups.
	// 4. Assigns the flow to the selected NIC queue. Even if packets
	// get queued up at the NIC queue for a while.
	if sg = c.getFreeSGroup(); sg != nil {
		go sg.worker.createSGroup(sg, dag)
		return sg.worker.switchPort, DefaultDstMACs[sg.pcieIdx], nil
	}

	// All active SGroups are running heavily. No free SGroups
	// are available. Just drop the packet. (Ideally, we should
	// never reach here if the cluster has enough resources.)
	return 0, "none", errors.New(fmt.Sprintf("No enough resources"))
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
		if sg.GetQLoad() > 40 || sg.GetPktLoad() > 80 {
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

// Selects the first SGroup from a worker w/o backgroun traffic.
func (c *FaaSController) findAvailableSGroupOnIdleWorker(dstPort uint32) *SGroup {
	var selected *SGroup = nil

	// Note: node1 starts at dstPort 1000
	workerName := fmt.Sprintf("node%d", dstPort-999)
	if w, ok := c.workers[workerName]; ok {
		if len(w.sgroups) > 0 {
			selected = w.sgroups[0]
		}
	}
	return selected
}
