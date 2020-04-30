package controller

import (
	"os"
	"sync"
	"testing"
	"time"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

var c *FaaSController = nil

func TestMain(m *testing.M) {
	c = NewFaaSController(true)
	// TODO(Jianfeng): set a fixed timeout on the instance startup time.
	go grpc.NewGRPCServer(c)

	retVal := m.Run()
	os.Exit(retVal)
}

func TestStartFreeSGroups(t *testing.T) {
	err := c.CleanUpAllWorkers()
	if err != nil {
		t.Errorf("Fail to reset FaaSController")
	}

	node := "ubuntu"
	w := c.workers[node]
	go w.CreateFreeSGroups(w.op)

	countSGroups := 10
	for i := 0; i < countSGroups; i++ {
		w.op <- FREE_SGROUP
	}

	start := time.Now()
	for time.Now().Unix()-start.Unix() < 60 && len(w.freeSGroups) != countSGroups {
		time.Sleep(100 * time.Millisecond)
	}

	if len(w.freeSGroups) != countSGroups {
		t.Errorf("Fail to create enough free SGroups")
	}

	// Cleanup.
	w.op <- SHUTDOWN
	w.cleanUp()

	if len(w.freeSGroups) != 0 {
		t.Errorf("Fail to clean up all free SGroups")
	}
}

func TestStartNFChain(t *testing.T) {
	err := c.CleanUpAllWorkers()
	if err != nil {
		t.Errorf("Fail to reset FaaSController")
	}

	node := "ubuntu"
	w := c.workers[node]
	go w.createFreeSGroup()

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

	var wg sync.WaitGroup
	go w.destroyFreeSGroup(sg, wg)
	wg.Wait()
}

// Tests for
func TestAttachAndDetachNFChain(t *testing.T) {

}
