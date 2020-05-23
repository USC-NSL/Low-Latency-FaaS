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
// |cycle| is the average per batch cycle.
// |incQueueLength| is the queue length of incQueue.
// |pktRateKpps| describes the observed traffic.
// |podName| is the Pod's deployment name in Kubernetes.
// |groupID| is the SGroup's ID.

type Instance struct {
	grpc.InstanceGRPCHandler
	funcType       string
	isNF           bool
	port           int
	address        string
	podName        string
	sg             *SGroup
	tid            int
	cycle          int
	incQueueLength int
	pktRateKpps    int
	cond           *sync.Cond
	mutex          sync.Mutex
}

func newInstance(funcType string, hostIp string, port int, podName string) *Instance {
	instance := Instance{
		funcType:       funcType,
		isNF:           (funcType != "prim" && funcType != "sched"),
		port:           port,
		address:        hostIp + ":" + strconv.Itoa(port),
		podName:        podName,
		tid:            kUninitializedTid,
		cycle:          0,
		incQueueLength: 0,
		pktRateKpps:    0,
		cond:           sync.NewCond(&sync.Mutex{}),
	}
	return &instance
}

func (ins *Instance) String() string {
	ins.mutex.Lock()
	defer ins.mutex.Unlock()

	return fmt.Sprintf("%s[port=%d, cycles=%d, qlen=%d, pps=%d]", ins.funcType, ins.port, ins.cycle, ins.incQueueLength, ins.pktRateKpps)
}

func (ins *Instance) ID() int {
	ins.mutex.Lock()
	defer ins.mutex.Unlock()

	return ins.port
}

func (ins *Instance) setCycles(cyclesPerPacket int) error {
	if !ins.InstanceGRPCHandler.IsConnEstablished() {
		if err := ins.InstanceGRPCHandler.EstablishConnection(ins.address); err != nil {
			return err
		}
	}
	_, err := ins.SetCycles(cyclesPerPacket)
	return err
}

func (ins *Instance) setBatch(batchSize int, batchNumber int) (string, error) {
	if !ins.InstanceGRPCHandler.IsConnEstablished() {
		if err := ins.InstanceGRPCHandler.EstablishConnection(ins.address); err != nil {
			return "", err
		}
	}
	response, err := ins.SetBatch(batchSize, batchNumber)
	msg := response.GetError().GetErrmsg()
	return msg, err
}

func (ins *Instance) updateTrafficInfo(qlen int, kpps int, cycle int) {
	ins.mutex.Lock()
	defer ins.mutex.Unlock()

	ins.incQueueLength = qlen
	ins.pktRateKpps = kpps
	ins.cycle = cycle
}

func (ins *Instance) getQlen() int {
	ins.mutex.Lock()
	defer ins.mutex.Unlock()

	return ins.incQueueLength
}

func (ins *Instance) getPktRate() int {
	ins.mutex.Lock()
	defer ins.mutex.Unlock()

	return ins.pktRateKpps
}

func (ins *Instance) getCycle() int {
	ins.mutex.Lock()
	defer ins.mutex.Unlock()

	return ins.cycle
}
