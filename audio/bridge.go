package audio

import (
	"fmt"
	"log"
	"sync"

	"github.com/Reverse-Call-Center/virtual-call-center/types"
)

// AudioBridge manages bidirectional audio between agent and caller
type AudioBridge struct {
	session        *types.CallSession
	agentExtension string
	isActive       bool
	stopChan       chan struct{}
	mutex          sync.RWMutex
}

var (
	activeBridges = make(map[string]*AudioBridge)
	bridgesMutex  sync.RWMutex
)

// StartAudioBridge starts bidirectional audio bridging between agent and caller
func StartAudioBridge(session *types.CallSession, agentExtension string) error {
	bridgesMutex.Lock()
	defer bridgesMutex.Unlock()

	log.Printf("Starting audio bridge for call %s with agent %s", session.ID, agentExtension)

	// Check if bridge already exists
	if _, exists := activeBridges[session.ID]; exists {
		return fmt.Errorf("audio bridge already exists for call %s", session.ID)
	}

	bridge := &AudioBridge{
		session:        session,
		agentExtension: agentExtension,
		isActive:       true,
		stopChan:       make(chan struct{}),
	}

	activeBridges[session.ID] = bridge
	log.Printf("Audio bridge started for call %s", session.ID)
	return nil
}

// StopAudioBridge stops the audio bridge for a call
func StopAudioBridge(callID string) {
	bridgesMutex.Lock()
	defer bridgesMutex.Unlock()

	bridge, exists := activeBridges[callID]
	if !exists {
		return
	}

	bridge.mutex.Lock()
	if bridge.isActive {
		bridge.isActive = false
		close(bridge.stopChan)
	}
	bridge.mutex.Unlock()

	delete(activeBridges, callID)
	log.Printf("Audio bridge stopped for call %s", callID)
}

// SendPCMToSIP converts PCM audio from agent and sends it to SIP caller
func SendPCMToSIP(callID string, pcmData []byte) error {
	bridgesMutex.RLock()
	bridge, exists := activeBridges[callID]
	bridgesMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no audio bridge found for call %s", callID)
	}

	bridge.mutex.RLock()
	defer bridge.mutex.RUnlock()

	if !bridge.isActive {
		return fmt.Errorf("audio bridge not active for call %s", callID)
	}

	log.Printf("Received %d bytes of PCM audio from agent %s for call %s",
		len(pcmData), bridge.agentExtension, callID)

	return nil
}

// SendPCMToSIPByAgent finds active bridge by agent extension and sends PCM audio
func SendPCMToSIPByAgent(agentExtension string, pcmData []byte) error {
	bridgesMutex.RLock()
	defer bridgesMutex.RUnlock()

	for callID, bridge := range activeBridges {
		if bridge.agentExtension == agentExtension && bridge.isActive {
			log.Printf("Found audio bridge for agent %s using call %s", agentExtension, callID)
			log.Printf("Received %d bytes of PCM audio from agent %s for call %s",
				len(pcmData), bridge.agentExtension, callID)
			return nil
		}
	}

	return fmt.Errorf("no active audio bridge found for agent %s", agentExtension)
}
