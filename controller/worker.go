
package controller

import (
	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
)

// The abstraction of worker node.
// |VSwitchGRPCHandler| and |ExecutorGRPCHandler| are functions to handle gRPC requests to vSwitch and executor on the node.
// |name| is the name of the node in kubernetes.
// |ip| is the ip address of the node.
// |vSwitchPort| is the port for vSwitch to establish gRCP server on the node.
// |executorPort| is the port of executor to establish gRCP server on the node.
// |cores| is the abstraction of cores on the node.
// |coreNumOffset| is designed for mapping from cores array to real physical core number.
//                 Specifically, coreNumOffset + index (in cores array) = real core number.
// |freeInstances| are the NF instances not assigned to any core yet (but still in memory).
// |instancePortPool| manages the ports taken by instances on the node to avoid port conflicts.
type Worker struct {
	grpc.VSwitchGRPCHandler
	grpc.ExecutorGRPCHandler
	name string
	ip string
	vSwitchPort int
	executorPort int
	cores []*Core
	coreNumOffset int
	freeInstances []*Instance
	instancePortPool *utils.IndexPool
}

func newWorker(name string, ip string, vSwitchport int, executorPort, coreNumOffset int, coreNum int) *Worker {
	worker := Worker{
		name: name,
		ip: ip,
		vSwitchPort: vSwitchport,
		executorPort: executorPort,
		cores: make([]*Core, coreNum),
		coreNumOffset: coreNumOffset,
		freeInstances: make([]*Instance, 0),
		// Ports taken by instances are between [50052, 51051]
		instancePortPool: utils.NewIndexPool(50052, 1000),
	}

	for i := 0; i < coreNum; i++ {
		worker.cores[i] = newCore()
	}
	return &worker
}
