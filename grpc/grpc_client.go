package grpc

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"time"
)

const (
	kGrpcConnTimeout = 5 * time.Second
	kGrpcReqTimeout  = 1 * time.Second
)

// A struct to establish or close a connection to a gRPC server.
type GRPCClient struct {
	grpcConn *grpc.ClientConn
}

func (client *GRPCClient) IsConnEstablished() bool {
	return client.grpcConn != nil
}

// Starts up a connection to gRPC server with |address|.
// |address| is a string in the form of "IP:Port".
func (client *GRPCClient) EstablishConnection(address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), kGrpcConnTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
	client.grpcConn = conn

	if err != nil {
		return errors.New("Failed to establish a connection with " + address)
	}
	return nil
}

// Ends the client connection to the target gRPC server.
func (client *GRPCClient) CloseConnection() error {
	if client.grpcConn == nil {
		return errors.New("attempt to close a empty connection")
	}

	error := client.grpcConn.Close()
	return error
}
