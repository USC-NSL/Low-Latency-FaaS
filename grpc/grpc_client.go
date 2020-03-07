
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

// Set up a connection to gRPC server with the address (ip:port).
func (client *GRPCClient) establishConnection(address string) error {
	// Add context for gRPC request to set timeout to three seconds
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
	client.grpcConn = conn

	if err != nil {
		return errors.New("fail to establish the connection with " + address)
	}
	return nil
}

// Close the connection.
func (client *GRPCClient) closeConnection() error {
	if client.grpcConn == nil {
		return errors.New("attempt to close a empty connection")
	}
	error := client.grpcConn.Close()
	return error
}
