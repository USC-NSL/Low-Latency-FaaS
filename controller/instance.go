package controller

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
	glog "github.com/golang/glog"
)

// Note: |Instance| is not a thread-safe type! All instances are
// managed and updated by SGroup, which is a thread-safe type.
// DO NOT EXPORT |Instance|.

// Default value for tid of an uninitialized instance.
const (
	kUninitializedTid = -1

	kMaxRpcConnTrials = 3
	// The max number of trials of making a gRPC call to an instance
	kMaxRpcCallTrials = 5
)

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
	profiledCycle  int
	cycle          int
	incQueueLength int
	pktRateKpps    int
	cond           *sync.Cond
	mutex          sync.Mutex
	backoff        *utils.Backoff
}

func newInstance(funcType string, cycleCost int, hostIp string, port int, podName string) *Instance {
	instance := Instance{
		funcType:       funcType,
		isNF:           (funcType != "prim" && funcType != "sched"),
		port:           port,
		address:        hostIp + ":" + strconv.Itoa(port),
		podName:        podName,
		tid:            kUninitializedTid,
		profiledCycle:  cycleCost,
		cycle:          0,
		incQueueLength: 0,
		pktRateKpps:    0,
		cond:           sync.NewCond(&sync.Mutex{}),
		backoff:        &utils.Backoff{Min: 100 * time.Millisecond, Max: 5 * time.Second, Factor: 2, Jitter: true},
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

func (ins *Instance) connect() error {
	ins.backoff.Reset()
	for try := 0; try < kMaxRpcConnTrials; try += 1 {
		err := ins.InstanceGRPCHandler.EstablishConnection(ins.address)
		if err == nil {
			return nil
		} else {
			glog.Warningf("Failed (trial=%s) to connect Instance %s. %v", try, ins.funcType, err)
		}
		time.Sleep(ins.backoff.Duration())
	}

	return fmt.Errorf("Failed all trials to connect Instance %s", ins.funcType)
}

func (ins *Instance) setCycles(cyclesPerPacket int) error {
	if !ins.InstanceGRPCHandler.IsConnEstablished() {
		if err := ins.connect(); err != nil {
			return err
		}
	}

	ins.backoff.Reset()
	for try := 0; try < kMaxRpcCallTrials; try += 1 {
		_, err := ins.SetCycles(cyclesPerPacket)
		if err == nil {
			return nil
		} else {
			glog.Warningf("Failed (trial=%d) to set cycle for Instance %s. %v", try, ins.funcType, err)
		}
		time.Sleep(ins.backoff.Duration())
	}

	return fmt.Errorf("Failed all trials to set cycle for Instance %s", ins.funcType)
}

func (ins *Instance) setBatch(batchSize int, batchNumber int) error {
	if !ins.InstanceGRPCHandler.IsConnEstablished() {
		if err := ins.connect(); err != nil {
			return err
		}
	}

	ins.backoff.Reset()
	for try := 0; try < kMaxRpcCallTrials; try += 1 {
		response, err := ins.SetBatch(batchSize, batchNumber)
		msg := response.GetError().GetErrmsg()

		if err == nil && msg == "" {
			return nil
		} else if err != nil {
			glog.Warningf("Failed (trial=%d) to set batch for Instance %s. %v", try, ins.funcType, err)
		} else {
			glog.Warningf("Error response: " + msg)
		}
		time.Sleep(ins.backoff.Duration())
	}

	return fmt.Errorf("Failed all trials to set batch for Instance %s", ins.funcType)
}

func (ins *Instance) UpdateTrafficInfo(qlen int, kpps int, cycle int) {
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
