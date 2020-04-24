package controller

import (
	"time"

	glog "github.com/golang/glog"
)

func (c *FaaSController) TestStartNFChain() bool {
	node := "ubuntu"
	dag := newDAG()
	dag.chains = append([]string{"chacha", "none", "acl"})
	var sg *SGroup = nil

	n := len(c.workers[node].freeSGroups)
	if n == 0 {
		return false
	}

	// Instantiates a |dag| at the SGroup |sg|.
	sg = c.workers[node].freeSGroups[n-1]
	w := c.workers[node]
	w.freeSGroups = w.freeSGroups[:(n - 1)]
	w.createSGroup(sg, dag)

	start := time.Now()
	for time.Now().Unix()-start.Unix() < 10 && len(sg.instances) != 3 {
		time.Sleep(1 * time.Second)
	}

	if len(sg.instances) != 3 {
		return false
	}

	elapsed := time.Now().Sub(start)
	glog.Infof("Time to deploy a NF chain: %s", elapsed.String())

	// Cleanup.
	c.workers[node].destroySGroup(sg)
	if len(sg.instances) != 0 {
		return false
	}

	c.workers[node].destroyFreeSGroup(sg)

	c.CleanUpAllWorkers()
	return true
}
