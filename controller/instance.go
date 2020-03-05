
package controller

import (
)

// The abstraction of NF instance.
// |funcType| is the type of NF inside the instance.
// |address| is the address (ip:port) of the gRPC server on the instance.
type Instance struct {
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
