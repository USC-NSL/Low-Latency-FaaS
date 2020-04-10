package controller

import (
	"fmt"
	"strconv"
	"sync"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

// The abstraction of NF instance.
// |InstanceGRPCHandler| are functions to handle gRPC requests to the instance.
// |funcType| is the type of NF inside the instance.
// |port| is the port of the gRPC server on the instance. It is thought as the unique id for an instance in the worker.
// |address| is the full address (ip:port) of the gRPC server on the instance.
// |tid| is the Tid of the instance in the worker.
// |cond| is the conditional variable used for tid initialization.
type Instance struct {
	grpc.InstanceGRPCHandler
	funcType string
	port     int
	address  string
	tid      int
	cond     *sync.Cond
}

func newInstance(funcType string, hostIp string, port int) *Instance {
	instance := Instance{
		funcType: funcType,
		port:     port,
		address:  hostIp + ":" + strconv.Itoa(port),
		tid:      0,
		cond:     sync.NewCond(&sync.Mutex{}),
	}
	return &instance
}

func (instance *Instance) String() string {
	return fmt.Sprintf("%s(port=%d,tid=%d)", instance.funcType, instance.port, instance.tid)
}

// When a instance is created, it will wait for the TID information sent through gRPC requests.
// Implemented based on conditional variable |cond| inside instance.
func (instance *Instance) waitTid() {
	instance.cond.L.Lock()
	for instance.tid == 0 {
		instance.cond.Wait()
	}
	instance.cond.L.Unlock()
}

// Wake up a instance and notify the tid when receiving its TID from gRPC request.
func (instance *Instance) notifyTid(tid int) {
	instance.cond.L.Lock()
	instance.tid = tid
	instance.cond.Signal()
	instance.cond.L.Unlock()
}

// Each worker has a pool to store instances which are waiting for the TID information sent through gRPC requests.
// |mutex| is required since both main thread and gRPC server thread may access it at the same time.
type InstanceWaitingPool struct {
	mutex sync.Mutex
	pool  []*Instance
}

// Add an instance in the waiting pool.
func (waitingPool *InstanceWaitingPool) add(instance *Instance) {
	waitingPool.mutex.Lock()
	waitingPool.pool = append(waitingPool.pool, instance)
	waitingPool.mutex.Unlock()
}

// Remove an instance (identified by its port) from the waiting pool after receiving its |tid|.
func (waitingPool *InstanceWaitingPool) remove(port int, tid int) {
	waitingPool.mutex.Lock()
	for i, instance := range waitingPool.pool {
		if instance.port == port {
			instance.notifyTid(tid)
			waitingPool.pool = append(waitingPool.pool[:i], waitingPool.pool[i+1:]...)
			waitingPool.mutex.Unlock()
			return
		}
	}
	waitingPool.mutex.Unlock()
	//TODO: Probably caused by competition.
	fmt.Printf("Error: Try to remove nonexistent instance with port %d", port)
}
