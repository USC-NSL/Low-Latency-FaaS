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

// This function adds a logical NF of |funcType| to DAG |g|.
// Returns an integral handler of this added NF.
func (g *DAG) addNF(funcType string) int {
	id := len(g.NFMap)
	g.NFMap[id] = &NF{
		id:       id,
		funcType: funcType,
		nextNFs:  make([]int, 0),
		prevNFs:  make([]int, 0),
	}
	return id
}

func (g *DAG) connectNFs(upID int, downID int) error {
	if upID < 0 || upID > len(g.NFMap) {
		return fmt.Errorf("Invalid NF |upID| (expect: [0, %d], input: %d)", len(g.NFMap), upID)
	}
	if downID < 0 || downID > len(g.NFMap) {
		return fmt.Errorf("Invalid NF |downID| (expect: [0, %d], input: %d)", len(g.NFMap), downID)
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

// This function activates a logical NF DAG |g|. First, it parses
// the logical DAG, and translates it into a set of logical SGroups.
// Then, it sets |g| as active, which indicates that this logical
// DAG is ready to serve traffic.
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
