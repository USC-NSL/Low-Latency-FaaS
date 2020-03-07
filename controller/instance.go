
package controller

import (
	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

// The abstraction of NF instance.
// |InstanceGRPCHandler| are functions to handle gRPC requests to the instance.
// |funcType| is the type of NF inside the instance.
// |address| is the address (ip:port) of the gRPC server on the instance.
type Instance struct {
	grpc.InstanceGRPCHandler
	funcType string
	address string
}

func newInstance(funcType string, address string) *Instance {
	instance := Instance{
		funcType: funcType,
		address: address,
	}
	return &instance
}
