package main

import (
	"bufio"
	"fmt"
	"os"

	cli "github.com/USC-NSL/Low-Latency-FaaS/cli"
	controller "github.com/USC-NSL/Low-Latency-FaaS/controller"
	grpc "github.com/USC-NSL/Low-Latency-FaaS/grpc"
	prompt "github.com/c-bata/go-prompt"
)

func main() {
	FaaSController := controller.NewFaaSController()
	go grpc.NewGRPCServer(FaaSController)
	e := cli.NewExecutor(FaaSController)

	p := prompt.New(
		e.Execute,
		cli.Complete,
		prompt.OptionPrefix(">>> "),
		prompt.OptionInputTextColor(prompt.Blue),
	)

	p.Run()
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
