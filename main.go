package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"testing"

	cli "github.com/USC-NSL/Low-Latency-FaaS/cli"
	controller "github.com/USC-NSL/Low-Latency-FaaS/controller"
	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	prompt "github.com/c-bata/go-prompt"
	glog "github.com/golang/glog"
)

func init() {
	testing.Init()
	flag.Usage = usage
	flag.Parse()
}

// By default, |-logtostderr| is false.
func usage() {
	fmt.Fprintf(os.Stderr, "usage: example -logtostderr=[true|false] -stderrthreshold=[INFO|WARNING|FATAL] -log_dir=[string]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	clusterInfo, err := controller.ParseClusterInfo("./cloudlab_cluster.json")
	if err != nil {
		glog.Errorf("Failed to read the cluster info. %v", err)
	}

	isTest := false
	FaaSController := controller.NewFaaSController(isTest, clusterInfo)
	go grpc.NewGRPCServer(FaaSController)
	e := cli.NewExecutor(FaaSController)

	p := prompt.New(
		e.Execute,
		cli.Complete,
		prompt.OptionPrefix(">>> "),
		prompt.OptionInputTextColor(prompt.Blue),
	)

	p.Run()
	glog.Flush()
}

func Prompt() {
	fmt.Printf("-> Press Return key to continue.")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		break
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	fmt.Println()
}
