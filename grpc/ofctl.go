package grpc

import (
	"fmt"
)

const (
	kMetronControlPlaneChan = "metronctl"
)

// The handler for sending gRPC requests to a vswitch.
// |GRPCClient| is the struct to maintain the gRPC connection.
type OfctlRpcHandler struct {
	RedisClient
}

// Metron control-plane functions
// Updates a SGroup (id, switch port, and dmac) to the OpenFlow controller.
func (h *OfctlRpcHandler) UpdateSGroup(sgID int, switchPort uint32, dmac string) error {
	msg := fmt.Sprintf("sgup,%d,%d,%s", sgID, switchPort, dmac)
	_, err := h.client.Publish(h.ctx, kMetronControlPlaneChan, msg).Result()
	return err
}

// Splits a SGroup into two. The new SGroup has already been registered
// at the controller. The switch controller splits the original traffic
// class into two.
func (h *OfctlRpcHandler) SpiltSGroup(firstID int, secondID int) error {
	msg := fmt.Sprintf("split,%d,%d", firstID, secondID)
	_, err := h.client.Publish(h.ctx, kMetronControlPlaneChan, msg).Result()
	return err
}

func (h *OfctlRpcHandler) UpdateAndSpiltSGroup(firstID int, sgID int, switchPort uint32, dmac string) error {
	msg := fmt.Sprintf("sgupsplit,%d,%d,%d,%s", firstID, sgID, switchPort, dmac)
	_, err := h.client.Publish(h.ctx, kMetronControlPlaneChan, msg).Result()
	return err
}

// Merges two SGroups into one. The second SGroup will no longer serve
// traffic. The switch controller migrates traffic classes from the
// second SGroup to the first one.
func (h *OfctlRpcHandler) MergeSGroup(firstID int, secondID int) error {
	msg := fmt.Sprintf("merge,%d,%d", firstID, secondID)
	_, err := h.client.Publish(h.ctx, kMetronControlPlaneChan, msg).Result()
	return err
}
