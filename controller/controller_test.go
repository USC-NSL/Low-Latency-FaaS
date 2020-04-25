package controller

import (
	"time"
	"testing"
)

var c *FaaSController = nil
var isTest bool = true

func TestMain(m *testing.M) {
	c = NewFaaSController(isTest)

	runTests := m.Run()
	os.Exit(runTests)
}

func (c *FaaSController) TestStartNFChain(t *testing.T) bool {
	node := "ubuntu"
	w := c.workers[node]
	w.createFreeSGroup()

	start := time.Now()
	for time.Now().Unix()-start.Unix() < 10 && len(w.freeSGroups) >= 1 {
		time.Sleep(100 * time.MilliSecond)
	}

	n := len(w.freeSGroups)
	if n == 0 {
		t.Errorf("Fail to create a free SGroup")
	}

	dag := newDAG()
	dag.chains = append([]string{"chacha", "none", "acl"})

	// Instantiates a |dag| at the SGroup |sg|.
	var sg *SGroup = w.freeSGroups[n-1]
	w.freeSGroups = w.freeSGroups[:(n - 1)]
	w.createSGroup(sg, dag)

	start = time.Now()
	for time.Now().Unix()-start.Unix() < 10 && len(sg.instances) != 3 {
		time.Sleep(1 * time.Second)
	}

	if len(sg.instances) != 3 {
		t.Errorf("Failed to deploy a NF DAG")
	}

	elapsed := time.Now().Sub(start)
	t.Logf("Time to deploy a NF chain: %s", elapsed.String())

	// Cleanup.
	w.destroySGroup(sg)
	if len(sg.instances) != 0 {
		t.Errorf("fail")
	}

	w.destroyFreeSGroup(sg)
	return true
}
