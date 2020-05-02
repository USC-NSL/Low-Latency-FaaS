package controller

import (
	"fmt"
	"strconv"
	"sync"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

// Note: |Instance| is not a thread-safe type! All instances are
// managed and updated by SGroup, which is a thread-safe type.
// DO NOT EXPORT |Instance|.

// Default value for tid of an uninitialized instance.
const kUninitializedTid int = -1

// The abstraction of NF instance.
// |InstanceGRPCHandler| are handlers for gRPC requests sent
// to instances.
// |funcType| is the NF type of the instance.
// |port| is the gRPC server's TCP port on the instance. It is
// an unique ID for the instance.
// |address| is the "ip:port" address of the gRPC server.
// |tid| is the thread ID of the instance.
// |podName| is the Pod's deployment name in Kubernetes.
// |groupID| is the SGroup's ID.

type Instance struct {
	funcType string
	port     int
	address  string
	podName  string
	sg       *SGroup
	tid      int
	grpc.InstanceGRPCHandler
	cond *sync.Cond
}

func newInstance(funcType string, hostIp string, port int, podName string) *Instance {
	instance := Instance{
		funcType: funcType,
		port:     port,
		address:  hostIp + ":" + strconv.Itoa(port),
		podName:  podName,
		tid:      kUninitializedTid,
		cond:     sync.NewCond(&sync.Mutex{}),
	}
	return &instance
}

func (ins *Instance) String() string {
	return fmt.Sprintf("%s[port=%d, tid=%d]", ins.funcType, ins.port, ins.tid)
}

func (ins *Instance) ID() int {
	return ins.port
}
