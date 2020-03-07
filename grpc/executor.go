
package grpc

import (
)

// The handler for sending gRPC requests to a executor on a machine.
// |GRPCClient| is the struct to maintain the gRPC connection.
type ExecutorGRPCHandler struct {
	GRPCClient
}
