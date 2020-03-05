
package grpc

import (
	"context"
	"fmt"
	"net"

	controller "github.com/USC-NSL/Low-Latency-FaaS/controller"
	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
	grpc "google.golang.org/grpc"
)

const (
	// Port for gRPC server.
	kGrpcPort = ":10515"
)

type FaaSControlServer struct {
	FaaSController		*controller.FaaSController
}

func NewGRPCServer(c *controller.FaaSController) {
	listen, err := net.Listen("tcp", kGrpcPort)
	if err != nil {
		fmt.Printf("Failed to listen: %v\n", err)
		return
	}

	s := grpc.NewServer()
	pb.RegisterFaaSControlServer(s, &FaaSControlServer{FaaSController: c})

	if err := s.Serve(listen); err != nil {
		fmt.Printf("Error: failed to start FaaS Server: %v\n", err)
	}
}

// Invoked when a new flow comes to system. Handled by UpdateFlow of FaaSController.
func (s *FaaSControlServer) UpdateFlow(context context.Context, flowInfo *pb.FlowInfo) (*pb.FlowTableEntry, error) {
	s.FaaSController.UpdateFlow(flowInfo.Ipv4Src, flowInfo.TcpSport, flowInfo.Ipv4Dst, flowInfo.TcpDport, flowInfo.Ipv4Protocol)
	response := &pb.FlowTableEntry{
	}
	return response, nil
}
