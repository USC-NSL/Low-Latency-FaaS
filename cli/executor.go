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
	FaaSController *controller.FaaSController
}

func NewExecutor(FaaSController *controller.FaaSController) *Executor {
	return &Executor{
		FaaSController: FaaSController,
	}
}

//---------------------------------------------------------
// TODO: Add more commands.
// The list of all API commands:
// 1. Query and Print: pods, deps, nodes.
// 2. List all the information of workers in the system: workers.
// 3. Create a sGroup on a node by a list of NFs:
//    - add |nodeName| |funcType1| |funcType2| ...
// 4. Remove a free sGroup on a node:
//    - rm |nodeName| |groupId|
// 5. Attach a sGroup to a core:
//    - attach |nodeName| |groupId| |coreId|
// 6. Detach a sGroup from a core:
//    - detach |nodeName| |groupId| |coreId|
// 7. Destroy a deployment in kubernetes by its name:
//    - kubectl rm |deploymentName|
// 8. Simulate a flow coming to the system:
//    - flow |srcIp| |srcPort| |dstIp| |dstPort| |protocol|
// 9. Set cycle parameters for a Bypass module:
//    - cycle |nodeName| |port| |cyclePerPacket|
// 10. Set batch size and number for an NF:
//    - batch |nodeName| |port| |batchSize| |batchNumber|
//---------------------------------------------------------
func (e *Executor) Execute(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	} else if s == "quit" {
		if err := e.FaaSController.Close(); err != nil {
			fmt.Printf("Failed to exit: %s\n", err.Error())
		}

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
	} else if words[0] == "workers" {
		if len(words) > 1 {
			name := words[1]
			fmt.Printf(e.FaaSController.GetWorkerInfoByName(name))
		} else {
			fmt.Printf(e.FaaSController.GetWorkersInfo())
		}
	} else if words[0] == "add" && len(words) >= 3 {
		nodeName := words[1]
		nfs := words[2:]

		e.FaaSController.CreateSGroup(nodeName, nfs)
	} else if words[0] == "rm" && len(words) >= 3 {
		nodeName := words[1]
		groupID, _ := strconv.Atoi(words[2])

		e.FaaSController.DestroySGroup(nodeName, groupID)
	} else if words[0] == "attach" && len(words) >= 4 {
		nodeName := words[1]
		groupId, _ := strconv.Atoi(words[2])
		coreId, _ := strconv.Atoi(words[3])
		if err := e.FaaSController.AttachSGroup(nodeName, groupId, coreId); err != nil {
			fmt.Printf("Failed to attach sGroup (id=%d) on core %d of worker %s: %s!\n", groupId, coreId, nodeName, err.Error())
		}
	} else if words[0] == "detach" && len(words) >= 3 {
		nodeName := words[1]
		groupId, _ := strconv.Atoi(words[2])

		if err := e.FaaSController.DetachSGroup(nodeName, groupId); err != nil {
			fmt.Printf("Failed to detach sGroup (id=%d) on worker %s: %s!\n", groupId, nodeName, err.Error())
		}
	} else if words[0] == "kubectl" && len(words) >= 3 {
		command := words[1]
		deploymentName := words[2]
		if command == "rm" {
			if err := kubectl.K8sHandler.DeleteDeployment(deploymentName); err != nil {
				fmt.Printf("Failed to remove deployment %s: %s.\n", deploymentName, err.Error())
			} else {
				fmt.Printf("Remove deployment %s successfully!\n", deploymentName)
			}
		}
	} else if words[0] == "flow" && len(words) >= 6 {
		srcIp := words[1]
		srcPort, _ := strconv.Atoi(words[2])
		dstIp := words[3]
		dstPort, _ := strconv.Atoi(words[4])
		protocol, _ := strconv.Atoi(words[5])
		if switchPort, dmac, err := e.FaaSController.UpdateFlow(srcIp, dstIp, uint32(srcPort), uint32(dstPort), uint32(protocol)); err != nil {
			fmt.Printf("Failed to update flow: %s!\n", err.Error())
		} else {
			fmt.Printf("Return switch port = %d, dmac = %s.", switchPort, dmac)
		}
	} else if words[0] == "deploy" && len(words) >= 3 {
		user := words[1]
		funcType := words[2]
		nfID := e.FaaSController.AddNF(user, funcType)

		fmt.Printf("User %s: add NF [%s], ID=%d.\n", user, funcType, nfID)
	} else if words[0] == "connect" && len(words) >= 4 {
		user := words[1]
		up, _ := strconv.Atoi(words[2])
		down, _ := strconv.Atoi(words[3])
		if err := e.FaaSController.ConnectNFs(user, up, down); err != nil {
			fmt.Println(err)
		}

		e.FaaSController.ShowNFDAGs(user)
	} else if words[0] == "show" && len(words) >= 2 {
		user := words[1]
		e.FaaSController.ShowNFDAGs(user)
	} else if words[0] == "activate" && len(words) >= 2 {
		user := words[1]
		e.FaaSController.ActivateDAG(user)
	} else if words[0] == "exp" {
		if len(words) == 2 {
			// For testing only, packets always have a dstPort 8080.
			if words[1] == "a" {
				user := "exp-a"
				nf2 := e.FaaSController.AddDummyNF(user, "acl")
				nf1 := e.FaaSController.AddDummyNF(user, "vlanpush")
				e.FaaSController.ConnectNFs(user, nf1, nf2)
				e.FaaSController.AddFlow(user, "", "", 0, 8080, 0)
				e.FaaSController.ActivateDAG(user)
			} else if words[1] == "b" {
				user := "exp-b"
				nf1 := e.FaaSController.AddDummyNF(user, "acl")
				nf2 := e.FaaSController.AddDummyNF(user, "urlfilter")
				nf3 := e.FaaSController.AddDummyNF(user, "chacha")
				e.FaaSController.ConnectNFs(user, nf1, nf2)
				e.FaaSController.ConnectNFs(user, nf2, nf3)
				e.FaaSController.AddFlow(user, "", "", 0, 8080, 0)
				e.FaaSController.ActivateDAG(user)
			} else if words[1] == "c" {
				user := "exp-c"
				nf1 := e.FaaSController.AddDummyNF(user, "acl")
				nf2 := e.FaaSController.AddNF(user, "nat")
				e.FaaSController.ConnectNFs(user, nf1, nf2)
				e.FaaSController.AddFlow(user, "", "", 0, 8080, 0)
				e.FaaSController.ActivateDAG(user)
			}
		} else {
			fmt.Println("Usage: exp [a|b|c]")
		}
	} else if words[0] == "cycle" && len(words) >= 4 {
		nodeName := words[1]
		port, _ := strconv.Atoi(words[2])
		cyclesPerPacket, _ := strconv.Atoi(words[3])
		if err := e.FaaSController.SetCycles(nodeName, port, cyclesPerPacket); err != nil {
			fmt.Printf("Failed to set cycles on instance (port=%d) of worker %s: %s!\n", port, nodeName, err.Error())
		}
	} else if words[0] == "batch" && len(words) >= 5 {
		nodeName := words[1]
		port, _ := strconv.Atoi(words[2])
		batchSize, _ := strconv.Atoi(words[3])
		batchNumber, _ := strconv.Atoi(words[4])
		if err := e.FaaSController.SetBatch(nodeName, port, batchSize, batchNumber); err != nil {
			fmt.Printf("Failed to set batch size on instance (port=%d) of worker %s: %s!\n", port, nodeName, err.Error())
		}
	}
}
