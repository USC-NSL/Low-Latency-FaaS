package grpc

import (
	"context"
	"errors"
	"time"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
)

// The handler for sending gRPC requests to a NF instance.
// |GRPCClient| is the struct to maintain the gRPC connection.
type InstanceGRPCHandler struct {
	GRPCClient
}

// Send gRPC request to collect statistics of the traffic class in the instance,
// including timestamp, count, cycles, packets and bits.
// Refer to proto/grpc_client.proto for the information of response.
func (handler *InstanceGRPCHandler) GetTCStatsForInstance(address string) (*pb.GetTcStatsResponse, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewInstanceControlClient(handler.grpcConn)
	response, err := client.GetTcStats(ctx, &pb.EmptyArg{})
	return response, err
}

// Send gRPC request to collect statistics of the queues that the instance reads packets from,
// including length and capacity for both inc and out queues.
// Refer to proto/grpc_client.proto for the information of response.
func (handler *InstanceGRPCHandler) GetPortQueueStatsForInstance(address string) (*pb.GetPortQueueStatsResponse, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewInstanceControlClient(handler.grpcConn)
	response, err := client.GetPortQueueStats(ctx, &pb.EmptyArg{})
	return response, err
}
