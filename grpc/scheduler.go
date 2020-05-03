package grpc

import (
	"context"
	"errors"
	"time"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
)

// gRPC Handlers for sending requests to a worker's CooperativeSched.
// |GRPCClient| maintains a gRPC connection to the server.
type SchedulerGRPCHandler struct {
	GRPCClient
}

// Registers a SGroup in the free threads pool on the worker.
// SGroup is managed by the scheduler and in a detached state.
func (handler *SchedulerGRPCHandler) SetupChain(tids []int32) (*pb.Error, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	res, err := client.SetupChain(ctx, &pb.SetupChainArg{Chain: tids})
	return res, err
}

// Unschedules a SGroup from the CooperativeSched on the worker.
// All futher operations won't be effective on this SGroup unless
// FaaSController registers the SGroup again.
func (handler *SchedulerGRPCHandler) RemoveChain(tids []int32) (*pb.Error, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	res, err := client.RemoveChain(ctx, &pb.RemoveChainArg{Chain: tids})
	return res, err
}

// Migrates/Schedules a SGroup on the worker's |core|.
func (handler *SchedulerGRPCHandler) AttachChain(tids []int32, core int) (*pb.Error, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	res, err := client.AttachChain(ctx, &pb.AttachChainArg{Chain: tids, Core: int32(core)})
	return res, err
}

// Detaches a SGroup. The SGroup stops running, but is still
// managed by the worker's |core|.
func (handler *SchedulerGRPCHandler) DetachChain(tids []int32, core int) (*pb.Error, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	res, err := client.DetachChain(ctx, &pb.DetachChainArg{Chain: tids, Core: int32(core)})
	return res, err
}

// Shutdown the scheduler. Restores all managed NF threads to
// normal CFS preemptive threads.
func (handler *SchedulerGRPCHandler) KillSched() (*pb.EmptyResponse, error) {
	if handler.grpcConn == nil {
		return nil, errors.New("connection does not exist")
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSchedulerControlClient(handler.grpcConn)
	res, err := client.KillSched(ctx, &pb.EmptyRequest{})
	return res, err
}
