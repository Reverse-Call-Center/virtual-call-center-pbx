package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Reverse-Call-Center/virtual-call-center/config"
	"github.com/Reverse-Call-Center/virtual-call-center/handlers"
	"github.com/Reverse-Call-Center/virtual-call-center/server"
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
	server.StartSIPServer(ctx, config)
}
