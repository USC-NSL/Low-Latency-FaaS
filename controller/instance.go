package controller

import (
	"fmt"
	"strconv"
	"sync"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

// Default value for tid of an uninitialized instance.
const kUninitializedTid int = -1

// The abstraction of NF instance.
// |InstanceGRPCHandler| are functions that handle gRPC requests sent
// to instances.
// |funcType| is the NF type of the instance.
// |port| is the gRPC server's TCP port on the instance. It is also
// an unique id for the instance.
// |address| is the ip:port address of the gRPC server.
// |tid| is the thread ID of the instance.
// |podName| is the Pod's deployment name in Kubernetes.
// |groupID| is the SGroup's ID.
// |cond| is the conditional variable used for tid initialization.
type Instance struct {
	grpc.InstanceGRPCHandler
	funcType string
	port     int
	address  string
	podName  string
	sg       *SGroup
	tid      int
	cond     *sync.Cond
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

// An NF instance |ins| will wait for its |tid| sent from the NF
// thread inside the container, when the instance is created.
// Implemented based on conditional variable |cond| inside instance.
func (ins *Instance) waitTid() {
	ins.cond.L.Lock()
	for ins.tid == kUninitializedTid {
		ins.cond.Wait()
	}
	ins.cond.L.Unlock()
}

// This function will be called when receiving a gRPC request from
// the NF thread when the instance |ins| is created and running. It
// wakes up |ins| and sets |tid| up.
func (ins *Instance) updateTID(tid int) {
	ins.cond.L.Lock()
	ins.tid = tid
	ins.cond.Signal()
	ins.cond.L.Unlock()
}
