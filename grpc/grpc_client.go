
package grpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc"
	"time"

	pb "github.com/USC-NSL/Low-Latency-FaaS/proto"
)

const (
	// Traffic generator uses 4 CPU cores.
	kTrafficGenCores = 4
)

// TODO: Remove the connection from the map when an instance is deleted.
type GRPCController struct {
	// Connections to function instances.
	instanceAddressToConnMap 	map[string]*grpc.ClientConn
	// Connections to vSwitches.
	switchAddressToConnMap		map[string]*grpc.ClientConn
	// Connections to ToR switches.
	torSwitchAddressToConnMap	map[string]*grpc.ClientConn
}

func NewGRPCController() (*GRPCController) {
	return &GRPCController{
		instanceAddressToConnMap	: make(map[string]*grpc.ClientConn),
		switchAddressToConnMap		: make(map[string]*grpc.ClientConn),
		torSwitchAddressToConnMap	: make(map[string]*grpc.ClientConn),
	}
}

// Set up a connection to gRPC server.
func connectToGRPCServer(address string) (*grpc.ClientConn, error) {
	// Add context for gRPC request to set timeout to three seconds
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())

	if err != nil {
		return nil, errors.New("Fail to establish the connection with " + address)
	}
	return conn, nil
}

// Connects to a gRPC server in a function instance. Stores the 
// connection info in the gRPC Controller.
func (c *GRPCController) EstablishConnWithInstance(address string) error {
	conn, error := connectToGRPCServer(address)
	if error == nil {
		c.instanceAddressToConnMap[address] = conn
	}
	return error
}

// Connects to a switch gRPC server. Stores the connection info
// in the gRPC Controller.
func (c *GRPCController) EstablishConnWithSwitch(address string) error {
	conn, error := connectToGRPCServer(address)
	if error == nil {
		c.switchAddressToConnMap[address] = conn
	}
	return error
}

// Set the default next function for an instance. If there is no entry in NFTable,
// all flows will next go to function (spi).
// Note: the specific (si) for that function will be determined by NFInstanceTable.
func (c *GRPCController) SetDefaultNextFunctionForInstance(address string, spi int) error {
	if _, exists := c.instanceAddressToConnMap[address]; !exists {
		return errors.New("Fail to connect with " + address)
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn := c.instanceAddressToConnMap[address]
	client := pb.NewInstanceControlClient(conn)

	_, err := client.SetDefaultNextFunction(ctx, &pb.NFTableEntry{
		Flow:                 nil,
		Spi:                  uint32(spi),
	})
	return err
}

// The NFTable is designed for the instance to handle multiple choices of next hop functions by
// deciding the type of next function based on the 5-tuple FlowInfo.
// (srcIP, dstIP, protocol, srcPort, dstPort) -> spi
// However, if there is only one possible next hop function, we can avoid using NFTable to reduce overhead.
func (c *GRPCController) SetNFTableEntryForInstance(address string, srcIP string, dstIP string,
	protocol uint32, srcPort uint32, dstPort uint32, spi int) error {
	if _, exists := c.instanceAddressToConnMap[address]; !exists {
		return errors.New("Fail to connect with " + address)
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.instanceAddressToConnMap[address]
	client := pb.NewInstanceControlClient(conn)
	flowInfo := pb.FlowInfo{
		Ipv4Src:              srcIP,
		Ipv4Dst:              dstIP,
		Ipv4Protocol:         protocol,
		TcpSport:             srcPort,
		TcpDport:             dstPort,
	}
	_, err := client.SetNFTableEntry(ctx, &pb.NFTableEntry{
		Flow:                 &flowInfo,
		Spi:                  uint32(spi),
	})
	return err
}

func (c *GRPCController) RemoveNFTableEntryForInstance(address string, srcIP string, dstIP string,
	protocol uint32, srcPort uint32, dstPort uint32, spi int) error {
	if _, exists := c.instanceAddressToConnMap[address]; !exists {
		return errors.New("Fail to connect with " + address)
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.instanceAddressToConnMap[address]
	client := pb.NewInstanceControlClient(conn)
	flowInfo := pb.FlowInfo{
		Ipv4Src:              srcIP,
		Ipv4Dst:              dstIP,
		Ipv4Protocol:         protocol,
		TcpSport:             srcPort,
		TcpDport:             dstPort,
	}
	_, err := client.RemoveNFTableEntry(ctx, &pb.NFTableEntry{
		Flow:                 &flowInfo,
		Spi:                  uint32(spi),
	})
	return err
}

// The NFInstanceTable is designed for the instance to find the next hop instance given the function type.
// (flowID, spi) -> si
// When a new flow comes, need to update NFInstanceTable for every instance on the possible NF path.
func (c *GRPCController) SetNFInstanceTableEntryForInstance(address string, flowId int, spi int, si int) error {
	if _, exists := c.instanceAddressToConnMap[address]; !exists {
		return errors.New("Fail to connect with " + address)
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.instanceAddressToConnMap[address]
	client := pb.NewInstanceControlClient(conn)
	_, err := client.SetNFInstanceTableEntry(ctx, &pb.NFInstanceTableEntry{FlowId: uint32(flowId), Spi: uint32(spi), Si: uint32(si)})
	return err
}

// When a flow is timeout, need to update NFInstanceTable for every instance on the possible NF path.
func (c *GRPCController) RemoveNFInstanceTableEntryForInstance(address string, flowId int, spi int, si int) error {
	if _, exists := c.instanceAddressToConnMap[address]; !exists {
		return errors.New("Fail to connect with " + address)
	}

	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.instanceAddressToConnMap[address]
	client := pb.NewInstanceControlClient(conn)
	_, err := client.RemoveNFInstanceTableEntry(ctx, &pb.NFInstanceTableEntry{FlowId: uint32(flowId), Spi: uint32(spi), Si: uint32(si)})
	return err
}

// Send gRPC request to instance at address [ip:port] to collect statistics of its traffic class,
// including timestamp, count, cycles, packets and bits.
// Refer to proto/grpc_client.proto for response information.
func (c *GRPCController) GetTCStatsForInstance(address string) (*pb.GetTcStatsResponse, error) {
	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.instanceAddressToConnMap[address]
	client := pb.NewInstanceControlClient(conn)
	response, err := client.GetTcStats(ctx, &pb.EmptyArg{})
	return response, err
}

// Send gRPC request to instance at address [ip:port] to collect statistics of the queue in the port,
// including length and capacity for both inc and out queues.
// Refer to proto/grpc_client.proto for response information.
func (c *GRPCController) GetPortQueueStatsForInstance(address string) (*pb.GetPortQueueStatsResponse, error) {
	// Add context for gRPC request to set timeout to one second
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.instanceAddressToConnMap[address]
	client := pb.NewInstanceControlClient(conn)
	response, err := client.GetPortQueueStats(ctx, &pb.EmptyArg{})
	return response, err
}

// Sends gRPC request to vSwitch with |address| (ip:port) to add 
// and update routing information.
func (c *GRPCController) UpdateRuleForSwitch(
		address string, spi int, si int, gate int) error {
	if _, exists := c.switchAddressToConnMap[address]; !exists {
		msg := fmt.Sprintf("Connection [%s] does not exist", address)
		return errors.New(msg)
	}

	// Avoids blocking. Waits for the server response for at most 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.switchAddressToConnMap[address]
	client := pb.NewBESSControlClient(conn)
	// Prepares arguments for updating the switch rule.
	_arg := &pb.NSHSwitchCommandAddArg{Spi: uint32(spi), Si: uint32(si), Gate: uint64(gate)}
	any, _ := ptypes.MarshalAny(_arg)

	res, err := client.ModuleCommand(ctx,
		&pb.CommandRequest{Name: "vswitch", Cmd: "add", Arg: any})

	if err != nil {
		fmt.Printf("RPC:", res)
	}

	return err
}

// Sends gRPC request to the traffic generator with |address| 
// (ip:port) to control the traffic volume.
// Note: |packetRate| must be larger than the number of flows.
// By default, there are about 4000 flows.
func (c *GRPCController) UpdateTrafficVolumeForSwitch(
		address string, packetRate int) error {
	if _, exists := c.switchAddressToConnMap[address]; !exists {
		msg := fmt.Sprintf("Connection [%s] does not exist", address)
		return errors.New(msg)
	}

	// Avoids blocking. Waits for the server response for at most 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.switchAddressToConnMap[address]
	client := pb.NewBESSControlClient(conn)
	// Prepares arguments for updating the switch rule.
	_arg := &pb.FlowGenArg{Pps: float64(packetRate / kTrafficGenCores)}
	any, _ := ptypes.MarshalAny(_arg)

	for i := 0; i < kTrafficGenCores; i++ {
		src := fmt.Sprintf("flowgen%d", i)
		res, err := client.ModuleCommand(ctx,
			&pb.CommandRequest{Name: src, Cmd: "update", Arg: any})
		if err != nil {
			fmt.Printf("RPC:", res)
			return err
		}
	}

	return nil
}

// Connects to a gRPC server in the ToR switch. Stores the
// connection info in the gRPC Controller.
func (c *GRPCController) EstablishConnWithToRSwitch(address string) error {
	conn, error := connectToGRPCServer(address)
	if error == nil {
		c.torSwitchAddressToConnMap[address] = conn
	}
	return error
}

// Sends gRPC request to ToR switch to set forwarding rule
// for instance [spi, si].
func (c *GRPCController) SetForwardingRuleForToRSwitch(
	address string, spi int, si int, port int) error {
	if _, exists := c.torSwitchAddressToConnMap[address]; !exists {
		msg := fmt.Sprintf("Connection [%s] does not exist", address)
		return errors.New(msg)
	}

	// Avoids blocking. Waits for the server response for at most 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.torSwitchAddressToConnMap[address]
	client := pb.NewSwitchControlClient(conn)
	_, err := client.SetForwardingRule(ctx,
		&pb.InstanceTableEntry{Spi: uint32(spi), Si: uint32(si), Port: uint32(port)})
	return err
}

// Sends gRPC request to ToR switch to remove forwarding rule
// for instance [spi, si].
func (c *GRPCController) RemoveForwardingRuleForToRSwitch(
	address string, spi int, si int, port int) error {
	if _, exists := c.torSwitchAddressToConnMap[address]; !exists {
		msg := fmt.Sprintf("Connection [%s] does not exist", address)
		return errors.New(msg)
	}

	// Avoids blocking. Waits for the server response for at most 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := c.torSwitchAddressToConnMap[address]
	client := pb.NewSwitchControlClient(conn)
	_, err := client.RemoveForwardingRule(ctx,
		&pb.InstanceTableEntry{Spi: uint32(spi), Si: uint32(si), Port: uint32(port)})
	return err
}
