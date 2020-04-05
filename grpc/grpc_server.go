
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
	UpdateFlow(srcIP string, srcPort uint32, dstIP string, dstPort uint32, protocol uint32) error
}

type GRPCServer struct {
	FaaSController		Controller
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

// Invoked when a new flow comes to system. Will forward to the system controller (with updateFlow method).
func (s *GRPCServer) UpdateFlow(context context.Context, flowInfo *pb.FlowInfo) (*pb.FlowTableEntry, error) {
	s.FaaSController.UpdateFlow(flowInfo.Ipv4Src, flowInfo.TcpSport, flowInfo.Ipv4Dst, flowInfo.TcpDport, flowInfo.Ipv4Protocol)
	response := &pb.FlowTableEntry{
	}
	return response, nil
}

// When a new instance sets up, it will inform the controller about its TID,
// which will be used by the scheduler on the machine to schedule the instance.
func (s *GRPCServer) InstanceSetUp(context context.Context, instanceInfo *pb.InstanceInfo) (*pb.Error, error) {
	fmt.Printf("Called by %d.\n", instanceInfo.GetTid())
	return &pb.Error{Code: 0}, nil
}
