package controller

import (
	"errors"
	"fmt"
	"strings"
)

type NF struct {
	id       int
	funcType string
	nextNFs  map[int]*NF
}

type DAG struct {
	NFMap map[int]*NF
}

func newDAG() *DAG {
	return &DAG{
		NFMap: make(map[int]*NF),
	}
}

// This function returns a formatted representation of NF graphs. The output
// is a string and used by graph-easy to visualize the graph.
func (g *DAG) String() string {
	dag := []string{}
	for id, nf := range g.NFMap {
		// Adds |nf|.
		currStr := fmt.Sprintf("[%s\\nid=%d]", nf.funcType, id)
		dag = append(dag, currStr)

		// Adds all edges.
		for nextId := range nf.nextNFs {
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
		nextNFs:  make(map[int]*NF),
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
	g.NFMap[upID].nextNFs[downID] = g.NFMap[downID]
	return nil
}
