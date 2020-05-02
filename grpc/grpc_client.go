package grpc

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"time"
)

// A struct to establish or close a connection to a gRPC server.
type GRPCClient struct {
	grpcConn *grpc.ClientConn
}

// Starts up a connection to gRPC server with |address|.
// |address| is a string in the form of "IP:Port".
func (client *GRPCClient) EstablishConnection(address string) error {
	// Add context for gRPC request to set timeout to three seconds
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
	client.grpcConn = conn

	if err != nil {
		return errors.New("fail to establish the connection with " + address)
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
