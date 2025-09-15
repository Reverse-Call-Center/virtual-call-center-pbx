package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"

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

	server.StartSIPServer(ctx, config, func() {
		go startHealthCheckServer(config.SIPPort + 1)
	})
}

func startHealthCheckServer(port int) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"virtual-call-center"}`))
	})

	addr := ":" + strconv.Itoa(port)
	fmt.Printf("Health check server listening on %s/health\n", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Printf("Health check server error: %v\n", err)
	}
}
