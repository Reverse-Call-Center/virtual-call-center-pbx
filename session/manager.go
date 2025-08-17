package session

import (
	"sync"

	"github.com/Reverse-Call-Center/virtual-call-center/types"
)

var (
	activeCalls map[string]*types.CallSession
	callsMutex  sync.RWMutex
)

func init() {
	activeCalls = make(map[string]*types.CallSession)
}

func RegisterCall(session *types.CallSession) {
	callsMutex.Lock()
	defer callsMutex.Unlock()
	activeCalls[session.ID] = session
}

func UnregisterCall(callID string) {
	callsMutex.Lock()
	defer callsMutex.Unlock()
	delete(activeCalls, callID)
}

func GetActiveCallCount() int {
	callsMutex.RLock()
	defer callsMutex.RUnlock()
	return len(activeCalls)
}

func GetCallsInState(state types.CallState) []*types.CallSession {
	callsMutex.RLock()
	defer callsMutex.RUnlock()

	var calls []*types.CallSession
	for _, call := range activeCalls {
		if call.State == state {
			calls = append(calls, call)
		}
	}
	return calls
}
