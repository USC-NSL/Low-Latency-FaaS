package controller

import (
	"testing"
	"time"
)

// Tests of creating a new worker and initializing all NIC queues.
func TestWorkerStartFreeSGroups(t *testing.T) {
	w := newWorker("ubuntu", "204.57.7.11", 10514, 10515, 1, 7)

	countSGroups := w.pciePool.Size()
	for i := 0; i < countSGroups; i++ {
		w.op <- FREE_SGROUP
	}

	start := time.Now()
	for time.Now().Unix()-start.Unix() < 30 && len(w.freeSGroups) != countSGroups {
		time.Sleep(100 * time.Millisecond)
	}

	if len(w.freeSGroups) != countSGroups {
		t.Errorf("Fail to create enough free SGroups")
	}

	// Cleanup.
	w.cleanUp()

	if len(w.freeSGroups) != 0 {
		t.Errorf("Fail to clean up all free SGroups")
	}
}

// Tests of deploying and deleting an NF DAG at a worker.
func TestStartNFChain(t *testing.T) {
	return
	w := newWorker("ubuntu", "204.57.7.11", 10514, 10515, 1, 7)
	w.op <- FREE_SGROUP

	start := time.Now()
	for time.Now().Unix()-start.Unix() < 20 && len(w.freeSGroups) != 1 {
		time.Sleep(100 * time.Millisecond)
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
		time.Sleep(100 * time.Millisecond)
	}

	if len(sg.instances) != len(dag.chains) {
		t.Errorf("Failed to deploy a NF DAG")
	}

	elapsed := time.Now().Sub(start)
	t.Logf("Time to deploy a NF chain: %s", elapsed.String())

	// Deletes instances.
	w.destroySGroup(sg)
	if len(sg.instances) != 0 {
		t.Errorf("fail")
	}

	// Cleanup.
	w.cleanUp()

	if len(w.freeSGroups) != 0 {
		t.Errorf("Fail to clean up all free SGroups")
	}
}

// Tests for Scheduling.
func TestAttachAndDetachNFChain(t *testing.T) {
}
