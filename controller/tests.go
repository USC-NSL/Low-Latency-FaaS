package controller

import (
	"time"
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

	sg = c.workers[node].freeSGroups[n - 1]
	w := c.workers[node]
	w.freeSGroups = w.freeSGroups[:(n - 1)]
	w.createSGroup(sg, dag)

	start := time.Now().Unix()
	startDone := false
	for time.Now().Unix() - start < 6 {
		if len(sg.instances) == 3 {
			startDone = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !startDone {
		return false
	}

	c.workers[node].destroySGroup(sg)

	if len(sg.instances) != 0 {
		return false
	}

	c.workers[node].destroyFreeSGroup(sg)
	return true
}
