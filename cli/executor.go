
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	controller "github.com/USC-NSL/Low-Latency-FaaS/controller"
	kubectl "github.com/USC-NSL/Low-Latency-FaaS/kubectl"
)

type Executor struct {
	FaaSController 	*controller.FaaSController
}

func NewExecutor(FaaSController *controller.FaaSController) *Executor {
	return &Executor {
		FaaSController : FaaSController,
	}
}

//---------------------------------------------------------
// TODO: Add more commands.
// The list of all API commands:
// 1. Query and Print: pods, deps, nodes.
// 2. List all the information of workers in the system: worker.
// 3. Create an instance on a node:
//    - add |nodeName| |funcType|
// 4. Remove an instance on a node:
//    - rm |nodeName| |funcType|
// 5. Destroy a deployment in kubernetes by its name:
//    - kubectl rm |deploymentName|
//---------------------------------------------------------
func (e *Executor) Execute(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	} else if s == "quit" {
		if err := e.FaaSController.CleanUpAllWorkers(); err != nil {
			fmt.Printf("Failed to exit: %s\n", err.Error())
		} else {
			os.Exit(0)
		}
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
	} else if words[0] == "worker" {
		if len(words) > 1 {
			name := words[1]
			fmt.Printf(e.FaaSController.GetWorkerInfoByName(name))
		} else {
			fmt.Printf(e.FaaSController.GetWorkersInfo())
		}
	} else if words[0] == "add" && len(words) > 2 {
		nodeName := words[1]
		funcType := words[2]
		if err := e.FaaSController.CreateInstance(nodeName, funcType); err != nil {
			fmt.Printf("Failed to create %s on %s: %s.\n", funcType, nodeName, err.Error())
		}
	} else if words[0] == "rm" && len(words) > 3 {
		nodeName := words[1]
		funcType := words[2]
		port, _ := strconv.Atoi(words[3])
		if err := e.FaaSController.DestroyInstance(nodeName, funcType, port); err != nil {
			fmt.Printf("Failed to delete %s on %s: %s.\n", funcType, nodeName, err.Error())
		}
	} else if words[0] == "kubectl" && len(words) > 2 {
		command := words[1]
		deploymentName := words[2]
		if command == "rm" {
			if err := kubectl.K8sHandler.DeleteDeploymentByName(deploymentName); err != nil {
				fmt.Printf("Failed to remove deployment %s: %s.\n", deploymentName, err.Error())
			} else {
				fmt.Printf("Remove deployment %s successfully!\n", deploymentName)
			}
		}
	}
}
