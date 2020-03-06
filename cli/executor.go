
package cli

import (
	"os"
	"strings"

	controller "github.com/USC-NSL/Low-Latency-FaaS/controller"
	kubectl "github.com/USC-NSL/Low-Latency-FaaS/kubectl"
)

type Executor struct {
	NFVController 	*controller.FaaSController
}

func NewExecutor(NFVController *controller.FaaSController) (*Executor) {
	return &Executor {
		NFVController : NFVController,
	}
}

//---------------------------------------------------------
// TODO: Add more commands.
// The list of all API commands:
// 1. Query and Print: pods, deps, nodes.
//---------------------------------------------------------
func (e *Executor) Execute(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	} else if s == "quit" {
		//kubectl.K8sHandlerResetAllFunctions()
		os.Exit(0)
	}

	words := strings.Fields(s)

	if words[0] == "pods" {
		kubectl.K8sHandler.FetchPods()
		kubectl.K8sHandler.PrintPods()
	} else if words[0] == "deps" {
		kubectl.K8sHandler.FetchDeployments()
		kubectl.K8sHandler.PrintDeployments()
	} else if words[0] == "nodes" {
		kubectl.K8sHandler.FetchNodes()
		kubectl.K8sHandler.PrintNodes()
	}
}
