package grpc

import (
	"context"
	"errors"
	"time"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
)

// The handler for sending gRPC requests to a scheduler on a machine.
// |GRPCClient| is the struct to maintain the gRPC connection.
type SchedulerGRPCHandler struct {
	GRPCClient
}

// Send gRPC request to set up a thread (identified by |tid|) in the free threads pool on the machine, but not schedule it.
func (handler *SchedulerGRPCHandler) SetUpThread(tid int) (*pb.Status, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	response, err := client.SetUpThread(ctx, &pb.SetUpThreadArg{Tid: int32(tid)})
	return response, err
}

// Send gRPC request to remove a sGroup (identified by an array of tid |tids|) from free threads pool and schedule it on |core|.
func (handler *SchedulerGRPCHandler) AttachChain(tids []int32, core int) (*pb.Status, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	response, err := client.AttachChain(ctx, &pb.AttachChainArg{Chain: tids, Core: int32(core)})
	return response, err
}

// Send gRPC request to migrate a sGroup (identified by an array of tid |tids|) from |coreFrom| to |coreTo|.
func (handler *SchedulerGRPCHandler) MigrateChain(tids []int32, coreFrom int, coreTo int) (*pb.Status, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	response, err := client.MigrateChain(ctx, &pb.MigrateChainArg{Chain: tids, CoreFrom: int32(coreFrom), CoreTo: int32(coreTo)})
	return response, err
}

// Send gRPC request to remove a sGroup (identified by an array of tid |tids|) from |core| and put it back to the free threads pool.
func (handler *SchedulerGRPCHandler) DetachChain(tids []int32, core int) (*pb.Status, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	response, err := client.DetachChain(ctx, &pb.DetachChainArg{Chain: tids, Core: int32(core)})
	return response, err
}
