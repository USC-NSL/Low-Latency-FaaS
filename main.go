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
	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
	prompt "github.com/c-bata/go-prompt"
	glog "github.com/golang/glog"
)

var clusterInfoFile string
var ctlOption string

func init() {
	flag.Usage = usage
	flag.StringVar(&clusterInfoFile, "cluster", "./cloudlab_cluster.json", "Specify the cluster node summary")
	flag.StringVar(&ctlOption, "ctl", "faas", "Select the cluster controller")

	if ctlOption != "faas" && ctlOption != "metron" && ctlOption != "nfvnice" {
		glog.Errorf("FaaSController does not support %s option", ctlOption)
		os.Exit(3)
	}

	testing.Init()
	flag.Parse()
}

// By default, |-logtostderr| is false.
func usage() {
	fmt.Fprintf(os.Stderr, "usage: example -logtostderr=[true|false] -stderrthreshold=[INFO|WARNING|FATAL] -log_dir=[string]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	clusterInfo, err := utils.ParseClusterInfo(clusterInfoFile)
	if err != nil {
		glog.Errorf("Failed to read the cluster info. %v", err)
	}

	isTest := false
	faasCtl := controller.NewFaaSController(isTest, ctlOption, clusterInfo)
	go grpc.NewGRPCServer(faasCtl)
	e := cli.NewExecutor(faasCtl)

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
