package grpc

import (
	"context"
	"fmt"
	"net"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"

	grpc "google.golang.org/grpc"
)

const (
	// Port for gRPC server.
	kGrpcPort = ":10515"
)

type Controller interface {
	UpdateFlow(srcIP string, srcPort uint32, dstIP string, dstPort uint32, protocol uint32) (string, error)
	// TODO: Add interface function for InstanceUpdateStats
	InstanceSetUp(nodeName string, port int, tid int) error
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
		fmt.Printf("Error: failed to start FaaS Server: %v\n", err)
	}
}

// This function is called when a new flow arrives at the ToR switch.
// The flow is updated to |s.FaaSController|, which then selects
// the target NF chains to process this flow.
func (s *GRPCServer) UpdateFlow(context context.Context, flowInfo *pb.FlowInfo) (*pb.FlowTableEntry, error) {
	dmac, err := s.FaaSController.UpdateFlow(flowInfo.Ipv4Src, flowInfo.TcpSport, flowInfo.Ipv4Dst, flowInfo.TcpDport, flowInfo.Ipv4Protocol)
	if err != nil {
		fmt.Printf("Error: failed to serve flow: %s\n", err.Error())
	}
	response := &pb.FlowTableEntry{SwitchPort: 20, Dmac: dmac}
	return response, err
}

// This function is called when a sgroup updates traffic statistics.
// |s.FaaSController| manages all sgroups, and is notified and updated.
func (s *GRPCServer) InstanceUpdateStats(context context.Context, stats *pb.SgroupStats) (*pb.EmptyArg, error) {
	respose := &pb.EmptyArg{}
	return respose, nil
}

// When a new instance sets up, it will inform the controller about its TID,
// which will be used by the scheduler on the machine to schedule the instance.
func (s *GRPCServer) InstanceSetUp(context context.Context, instanceInfo *pb.InstanceInfo) (*pb.Error, error) {
	nodeName := instanceInfo.GetNodeName()
	port := int(instanceInfo.GetPort())
	tid := int(instanceInfo.GetTid())
	fmt.Printf("Called by %s:%d tid=%d .\n", nodeName, port, tid)
	if err := s.FaaSController.InstanceSetUp(nodeName, port, tid); err != nil {
		return &pb.Error{Code: 1, Errmsg: err.Error()}, err
	}
	return &pb.Error{Code: 0}, nil
}
