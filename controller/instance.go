
package controller

import (
	"fmt"
	"strconv"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

// The abstraction of NF instance.
// |InstanceGRPCHandler| are functions to handle gRPC requests to the instance.
// |funcType| is the type of NF inside the instance.
// |port| is the port of the gRPC server on the instance. It is thought as the unique id for an instance in the worker.
// |address| is the full address (ip:port) of the gRPC server on the instance.
type Instance struct {
	grpc.InstanceGRPCHandler
	funcType string
	port int
	address string
}

func newInstance(funcType string, hostIp string, port int) *Instance {
	instance := Instance{
		funcType: funcType,
		port: port,
		address: hostIp + ":" + strconv.Itoa(port),
	}
	return &instance
}

func (instance *Instance) String() string {
	return fmt.Sprintf("%s(%d)", instance.funcType, instance.port)
}
