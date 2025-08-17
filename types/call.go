package types

import (
	"context"
	"time"

	"github.com/emiago/diago"
)

type CallSession struct {
	ID        string
	CallerID  string
	Dialog    *diago.DialogServerSession
	State     CallState
	IVRLevel  int
	QueueID   int
	AgentExt  string
	StartTime time.Time
	Context   context.Context
	Cancel    context.CancelFunc
}

type CallState int

const (
	StateConnecting CallState = iota
	StateIVR
	StateQueue
	StateConnected
	StateWithAgent
	StateHangup
)
