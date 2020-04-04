package grpc

import (
	"context"
	"errors"
	"time"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
)

// The handler for sending gRPC requests to a ToR switch.
// |GRPCClient| is the struct to maintain the gRPC connection.
type ToRGRPCHandler struct {
	GRPCClient
}

// Sends gRPC request to ToR switch to set forwarding rule for instance [spi, si].
func (handler *ToRGRPCHandler) SetForwardingRuleForToRSwitch(address string, spi int, si int, port int) error {
	if handler.grpcConn == nil {
		return errors.New("connection does not exist")
	}

	// Avoids blocking. Waits for the server response for at most 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSwitchControlClient(handler.grpcConn)
	_, err := client.SetForwardingRule(ctx,
		&pb.InstanceTableEntry{Spi: uint32(spi), Si: uint32(si), Port: uint32(port)})
	return err
}

// Sends gRPC request to ToR switch to remove forwarding rule for instance [spi, si].
func (handler *ToRGRPCHandler) RemoveForwardingRuleForToRSwitch(address string, spi int, si int, port int) error {
	if handler.grpcConn == nil {
		return errors.New("connection does not exist")
	}

	// Avoids blocking. Waits for the server response for at most 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pb.NewSwitchControlClient(handler.grpcConn)
	_, err := client.RemoveForwardingRule(ctx,
		&pb.InstanceTableEntry{Spi: uint32(spi), Si: uint32(si), Port: uint32(port)})
	return err
}
