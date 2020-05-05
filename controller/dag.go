package controller

import (
	"errors"
	"fmt"
	"strings"
)

// |NF| is the abstraction of logical NFs.
// All NFs are managed and read/written by DAG.
type NF struct {
	id       int
	funcType string
	prevNFs  []int
	nextNFs  []int
}

// |DAG| is the abstraction of NF DAG deployment specified by
// FaaS-NFV users. It defines a logical NF DAG that defines
// dependencies among NFs, and a set of |flowlets| that defines
// a set of traffic to be processed by this |DAG| deployment.
type DAG struct {
	NFMap    map[int]*NF
	flowlets []*flowlet
	chains   []string
	sgroups  []*SGroup
	isActive bool
}

func newDAG() *DAG {
	return &DAG{
		NFMap:    make(map[int]*NF),
		flowlets: make([]*flowlet, 0),
		chains:   make([]string, 0),
		sgroups:  make([]*SGroup, 0),
		isActive: false,
	}
}

// This function returns a formatted representation of NF graphs.
// Returns a string to visualize the graph via graph-easy.
func (g *DAG) String() string {
	dag := []string{}
	for id, nf := range g.NFMap {
		// Adds |nf|.
		currStr := fmt.Sprintf("[%s\\nid=%d]", nf.funcType, id)
		dag = append(dag, currStr)

		// Adds all edges.
		for _, nextId := range nf.nextNFs {
			nextNF := g.NFMap[nextId]
			nextStr := fmt.Sprintf("[%s\\nid=%d]", nextNF.funcType, nextId)
			dag = append(dag, currStr+" -> "+nextStr)
		}
	}
	return strings.Join(dag, " ")
}

func (g *DAG) addNF(funcType string) error {
	id := len(g.NFMap)
	g.NFMap[id] = &NF{
		id:       id,
		funcType: funcType,
		nextNFs:  make([]int, 0),
		prevNFs:  make([]int, 0),
	}
	return nil
}

func (g *DAG) connectNFs(up string, down string) error {
	upID, downID := -1, -1
	for id, nf := range g.NFMap {
		if nf.funcType == up {
			upID = id
		} else if nf.funcType == down {
			downID = id
		}
	}

	if upID == -1 || downID == -1 {
		return errors.New(fmt.Sprintf(
			"Error: failed to connect [%s] -> [%s]", up, down))
	}

	g.NFMap[upID].nextNFs = append(g.NFMap[upID].nextNFs, downID)
	g.NFMap[downID].prevNFs = append(g.NFMap[downID].prevNFs, upID)
	return nil
}

// Adds a new flowlet to |g|. Flows matched with this flowlet are
// processed by this logical DAG.
func (g *DAG) addFlow(srcIP string, dstIP string, srcPort uint32, dstPort uint32, proto uint32) {
	f := flowlet{srcIP, dstIP, srcPort, dstPort, proto}
	g.flowlets = append(g.flowlets, &f)
}

// Checks whether an incoming flow needs to be processed by |g|.
func (g *DAG) Match(srcIP string, dstIP string, srcPort uint32, dstPort uint32, proto uint32) bool {
	for _, f := range g.flowlets {
		if f.Match(srcIP, dstIP, srcPort, dstPort, proto) {
			return true
		}
	}
	return false
}

func (g *DAG) Activate() error {
	cnt := 0
	var ingress *NF = nil
	for _, nf := range g.NFMap {
		if len(nf.prevNFs) == 0 {
			ingress = nf
			cnt += 1
		}
	}

	if ingress == nil || cnt != 1 {
		return errors.New("Failed to active. Invalid ingress node.")
	}

	g.chains = nil
	// TODO: handle branching cases
	curr := ingress
	for {
		g.chains = append(g.chains, curr.funcType)

		if len(curr.nextNFs) == 0 {
			break
		}

		curr = g.NFMap[curr.nextNFs[0]]
	}

	g.isActive = true
	fmt.Printf("Activated chains: %v\n", g.chains)
	return nil
}
