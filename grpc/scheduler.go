package grpc

// The handler for sending gRPC requests to a scheduler on a machine.
// |GRPCClient| is the struct to maintain the gRPC connection.
type SchedulerGRPCHandler struct {
	GRPCClient
}
