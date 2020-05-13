package grpc

import (
	"context"
	"fmt"
	"net"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
	glog "github.com/golang/glog"
	grpc "google.golang.org/grpc"
)

const (
	// Port for gRPC server.
	kGrpcPort = ":10515"
)

type Controller interface {
	UpdateFlow(srcIP string, dstIP string, srcPort uint32, dstPort uint32, proto uint32) (string, error)

	InstanceSetUp(nodeName string, port int, tid int) error

	InstanceUpdateStats(nodeName string, port int, qlen int, kpps int, cycle int) error
}

type GRPCServer struct {
	FaaSController Controller
}

func NewGRPCServer(c Controller) {
	listen, err := net.Listen("tcp", kGrpcPort)
	if err != nil {
		fmt.Printf("Failed to listen: %v\n", err)
		return
	}

	s := grpc.NewServer()
	pb.RegisterFaaSControlServer(s, &GRPCServer{FaaSController: c})

	if err := s.Serve(listen); err != nil {
		glog.Errorf("Failed to start FaaS Server: %v\n", err)
	}
}

// This function is called when a new flow arrives at the ToR switch.
// |flowInfo| is the flow's 5-tuple. FaaSController assigns a active
// NF chain to process this flow.
func (s *GRPCServer) UpdateFlow(context context.Context, flowInfo *pb.FlowInfo) (*pb.FlowTableEntry, error) {
	dmac, err := s.FaaSController.UpdateFlow(flowInfo.Ipv4Src, flowInfo.Ipv4Dst,
		flowInfo.TcpSport, flowInfo.TcpDport, flowInfo.Ipv4Protocol)

	if err != nil {
		glog.Errorf("Failed to serve flow: %v", err)
		// TODO(Jianfeng): handle errors.
	}

	// TODO(Jianfeng): assign a switch port number to each worker.
	res := &pb.FlowTableEntry{SwitchPort: 20, Dmac: dmac}
	return res, err
}

// When a new instance sets up, it will inform the controller about its TID,
// which will be used by the scheduler on the machine to schedule the instance.
func (s *GRPCServer) InstanceSetUp(context context.Context, instanceInfo *pb.InstanceInfo) (*pb.Error, error) {
	nodeName := instanceInfo.GetNodeName()
	port := int(instanceInfo.GetPort())
	tid := int(instanceInfo.GetTid())
	glog.Infof("Set up Instance [w=%s, port=%d, tid=%d]\n", nodeName, port, tid)

	if err := s.FaaSController.InstanceSetUp(nodeName, port, tid); err != nil {
		glog.Errorf("Failed to set up Instance. %v", err)
		return &pb.Error{Code: 1, Errmsg: err.Error()}, err
	}

	return &pb.Error{Code: 0}, nil
}

// This function is called when a sgroup updates traffic statistics.
// |s.FaaSController| manages all sgroups, and is notified and updated.
func (s *GRPCServer) InstanceUpdateStats(context context.Context, msg *pb.TrafficInfo) (*pb.Error, error) {
	nodeName := msg.GetNodeName()
	port := int(msg.GetPort())
	qlen := int(msg.GetQlen())
	kpps := int(msg.GetKpps())
	cycle := int(msg.GetCycle())

	if err := s.FaaSController.InstanceUpdateStats(nodeName, port, qlen, kpps, cycle); err != nil {
		return &pb.Error{Code: 1, Errmsg: err.Error()}, err
	}

	return &pb.Error{Code: 0}, nil
}
