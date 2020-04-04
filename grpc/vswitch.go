package grpc

// The handler for sending gRPC requests to a vswitch.
// |GRPCClient| is the struct to maintain the gRPC connection.
type VSwitchGRPCHandler struct {
	GRPCClient
}
