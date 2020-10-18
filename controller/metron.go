package controller

import (
	"fmt"
	"sync"
	"time"

	glog "github.com/golang/glog"
	rand "math/rand"
)

const (
	kDefaultSGroupInStartup = 1
)

// This file contains important controller functions used by Metron.

func (c *FaaSController) UpdatePort(ports []uint32) ([]int32, error) {
	allSGs := make([]int32, 0)
	for _, p := range ports {
		for _, w := range c.workers {
			if w.switchPort == p {
				sgs := c.metronProcessWorker(w)
				allSGs = append(allSGs, sgs...)
			}
		}

		glog.Errorf("Port %d does not match any worker")
	}

	return allSGs, nil
}

// This function activates all inactive DAGs. It tries to bring up
// a certain number of NF chains in the cluster.
func (c *FaaSController) metronStartUp() {
	var wg sync.WaitGroup

	for _, dag := range c.dags {
		if dag.IsActive() {
			continue
		}

		for i := 0; i < kDefaultSGroupInStartup; i++ {
			wg.Add(1)
			go func(c *FaaSController, dag *DAG) {
				sg := c.metronGetFreeSGroup()
				if sg != nil {
					sg.worker.metronCreateSGroup(sg, dag)
					// Check that sg is up and then notify ofctl
					if sg.IsReady() {
						c.ofctlRpc.UpdateSGroup(sg.ID(), sg.worker.switchPort, DefaultDstMACs[sg.pcieIdx])
					}
				} else {
					glog.Errorf("Failed to create a new SGroup (no resources)")
				}

				wg.Done()
			}(c, dag)
		}
	}

	wg.Wait()
}

// This function implements Metron's algorithm of picking an idle
// core from the cluster.
func (c *FaaSController) metronGetFreeSGroup() *SGroup {
	num := len(c.workers)
	rand1 := rand.Intn(num)
	rand2 := rand.Intn(num)
	for rand2 == rand1 {
		rand2 = rand.Intn(num)
	}

	w1 := fmt.Sprintf("node%d", (1 + rand1))
	w2 := fmt.Sprintf("node%d", (1 + rand2))

	if c.workers[w1].GetPktLoad() > c.workers[w2].GetPktLoad() {
		return c.workers[w2].getFreeSGroup()
	}

	return c.workers[w1].getFreeSGroup()
}

func (c *FaaSController) metronProcessWorker(w *Worker) []int32 {
	sgs := make([]int32, 0)

	for _, sg := range w.sgroups {
		if sg.metronIsOverloaded() {
			sgs = append(sgs, int32(sg.groupID))
			c.metronScaleUp(sg)
		}
		glog.Infof("sg (%s) is affected", sg.groupID)
	}

	return sgs
}

func (sg *SGroup) metronIsOverloaded() bool {
	return sg.GetPktLoad() > 80
}

func (c *FaaSController) metronScaleUp(sg *SGroup) {
	newSGroup := c.metronGetFreeSGroup()
	if newSGroup == nil {
		return
	}

	// Create newSGroup that replicates sg.
	newSGroup.worker.metronCreateSGroup(newSGroup, sg.dag)

	// Wait for the new sg is up. Then, update to ofctl.
	start := time.Now()
	for time.Now().Unix()-start.Unix() < 20 {
		if newSGroup.IsReady() {
			c.ofctlRpc.UpdateAndSpiltSGroup(sg.ID(), newSGroup.ID(), newSGroup.worker.switchPort, DefaultDstMACs[newSGroup.pcieIdx])
			break
		}
	}
}
