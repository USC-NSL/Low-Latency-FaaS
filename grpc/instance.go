package grpc

import (
	"context"
	"errors"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
)

// The handler for sending gRPC requests to an NF instance.
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

	// Add gRPC context to set timeout for this request
	ctx, cancel := context.WithTimeout(context.Background(), kGrpcReqTimeout)
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

	// Add gRPC context to set timeout for this request
	ctx, cancel := context.WithTimeout(context.Background(), kGrpcReqTimeout)
	defer cancel()

	client := pb.NewInstanceControlClient(handler.grpcConn)
	response, err := client.GetPortQueueStats(ctx, &pb.EmptyArg{})
	return response, err
}

// Send gRPC request to update cycles for Bypass Module.
func (handler *InstanceGRPCHandler) SetCycles(cyclesPerPacket int) (*pb.EmptyArg, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add gRPC context to set timeout for this request
	ctx, cancel := context.WithTimeout(context.Background(), kGrpcReqTimeout)
	defer cancel()

	client := pb.NewInstanceControlClient(handler.grpcConn)
	response, err := client.SetCycles(ctx, &pb.BypassArg{
		CyclesPerBatch:  0,
		CyclesPerPacket: uint32(cyclesPerPacket),
		CyclesPerByte:   0,
	})
	return response, err
}

// Set batch size and batch number for NF.
// See message.proto for more information.
func (handler *InstanceGRPCHandler) SetBatch(batchSize int, batchNumber int) (*pb.CommandResponse, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add gRPC context to set timeout for this request
	ctx, cancel := context.WithTimeout(context.Background(), kGrpcReqTimeout)
	defer cancel()

	client := pb.NewInstanceControlClient(handler.grpcConn)
	response, err := client.SetBatchSize(ctx, &pb.SetBatchArg{
		BatchSize:   uint32(batchSize),
		BatchNumber: uint32(batchNumber),
	})
	return response, err
}
