package controller

import (
	"os"
	//"sync"
	"testing"

	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
)

var c *FaaSController = nil

func TestMain(m *testing.M) {
	c = NewFaaSController(true)
	// TODO(Jianfeng): set a fixed timeout on the instance startup time.
	go grpc.NewGRPCServer(c)

	ret := m.Run()
	os.Exit(ret)
}
