package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	"github.com/Reverse-Call-Center/virtual-call-center/agents"
	"github.com/Reverse-Call-Center/virtual-call-center/config"
	"github.com/Reverse-Call-Center/virtual-call-center/handlers"
	pb "github.com/Reverse-Call-Center/virtual-call-center/proto"
	"github.com/Reverse-Call-Center/virtual-call-center/server"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Println("Virtual Call Center Starting...")
	config, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	handlers.InitializeConfigs()

	go startAgentServer()

	server.StartSIPServer(ctx, config)
}

func startAgentServer() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAgentServiceServer(grpcServer, agents.NewAgentServer())

	fmt.Println("Agent gRPC server listening on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
