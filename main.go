package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/Reverse-Call-Center/virtual-call-center/configs"
	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Println("Virtual Call Center Starting...")
	config, err := configs.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	startSIPServer(config, ctx)
}

func startSIPServer(config *configs.Config, ctx context.Context) {
	fmt.Println("Starting SIP server on:", config.SIPProtocol, config.SIPListenAddress, ":", config.SIPPort)

	transport := diago.Transport{
		Transport: config.SIPProtocol,
		BindHost:  config.SIPListenAddress,
		BindPort:  config.SIPPort,
	}

	ua, err := sipgo.NewUA()
	if err != nil {
		fmt.Printf("Error creating SIP User Agent: %v\n", err)
		return
	}

	dg := diago.NewDiago(ua,
		diago.WithTransport(transport))

	dg.Serve(ctx, func(inDialog *diago.DialogServerSession) {
		// Handle incoming SIP dialog
		inDialog.Trying()
		inDialog.Answer()
		fromHeader := inDialog.InviteRequest.Headers()

		for _, header := range fromHeader {
			if header.Name() == "From" {
				fmt.Printf("Incoming call from: %s\n", extractCallerPhone(header.Value()))
			}
		}

		playFile, err := os.Open("example.wav")
		if err != nil {
			fmt.Printf("Error opening audio file: %v\n", err)
			return
		}
		defer playFile.Close()

		pb, err := inDialog.PlaybackCreate()
		if err != nil {
			fmt.Printf("Error creating playback: %v\n", err)
			return
		}

		_, err = pb.Play(playFile, "audio/wav")
		if err != nil {
			fmt.Printf("Error playing audio: %v\n", err)
			return
		}

		fmt.Println("Call from ", extractCallerPhone(fromHeader[0].Value()), "answered, playing audio...")
	})
}

func extractCallerPhone(from string) string {
	// Extract phone number from SIP From header assuming format <sip:number@domain>
	if strings.HasPrefix(from, "<sip:") {
		trimmed := strings.TrimPrefix(from, "<sip:")
		parts := strings.Split(strings.TrimSuffix(trimmed, ">"), "@")
		return parts[0]
	} else {
		fmt.Println("Invalid SIP From header format:", from)
	}
	return from
}
