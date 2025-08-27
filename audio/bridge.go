package audio

import (
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/Reverse-Call-Center/virtual-call-center/types"
)

// AgentAudioSender interface for sending audio to agents
type AgentAudioSender interface {
	SendAudioToAgent(extension string, callID string, audioData []byte) error
}

var agentManager AgentAudioSender

// SetAgentManager sets the agent manager for sending audio to agents
func SetAgentManager(am AgentAudioSender) {
	agentManager = am
}

// AudioBridge manages bidirectional audio between agent and caller
type AudioBridge struct {
	session        *types.CallSession
	agentExtension string
	isActive       bool
	stopChan       chan struct{}
	mutex          sync.RWMutex
	audioReader    io.Reader
	pcmPlayer      *StreamingPCMPlayer
	readerActive   bool
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
		return fmt.Errorf("audio bridge already exists for call %s", session.ID)	}

	// Get audio reader from dialog session
	audioReader, err := session.Dialog.AudioReader()
	if err != nil {
		return fmt.Errorf("failed to get audio reader for call %s: %v", session.ID, err)
	}

	// Create the PCM player for streaming audio to SIP caller
	pcmPlayer, err := NewStreamingPCMPlayer(session)
	if err != nil {
		return fmt.Errorf("failed to create PCM player for call %s: %v", session.ID, err)
	}

	bridge := &AudioBridge{
		session:        session,
		agentExtension: agentExtension,
		isActive:       true,
		stopChan:       make(chan struct{}),
		audioReader:    audioReader,
		pcmPlayer:      pcmPlayer,
		readerActive:   false,
	}

	activeBridges[session.ID] = bridge

	// Start listening for audio from SIP caller to send to agent
	go bridge.streamSIPToAgent()

	log.Printf("Audio bridge started for call %s with PCM player streaming", session.ID)
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
		
		// Stop the PCM player
		if bridge.pcmPlayer != nil {
			bridge.pcmPlayer.Stop()
		}
		
		log.Printf("Signaled stop for audio bridge %s", callID)
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

	// Stream PCM audio to SIP caller using audio bridge
	return bridge.streamPCMToSIP(pcmData)
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

			// Stream the PCM data to SIP
			return bridge.streamPCMToSIP(pcmData)
		}
	}

	return fmt.Errorf("no active audio bridge found for agent %s", agentExtension)
}

// streamPCMToSIP streams PCM audio data to the SIP caller
func (bridge *AudioBridge) streamPCMToSIP(pcmData []byte) error {
	if bridge.pcmPlayer == nil {
		return fmt.Errorf("PCM player not initialized for call %s", bridge.session.ID)
	}

	// Write PCM data to the streaming player
	err := bridge.pcmPlayer.WritePCM(pcmData)
	if err != nil {
		log.Printf("Error writing PCM data to player for call %s: %v", bridge.session.ID, err)
		return err
	}

	log.Printf("Successfully queued %d bytes of PCM audio for SIP caller for call %s", len(pcmData), bridge.session.ID)
	return nil
}

// streamSIPToAgent reads audio from SIP caller and sends it to agent
func (bridge *AudioBridge) streamSIPToAgent() {
	bridge.mutex.Lock()
	if !bridge.isActive {
		bridge.mutex.Unlock()
		return
	}
	bridge.readerActive = true
	bridge.mutex.Unlock()

	log.Printf("Started SIP-to-Agent audio streaming for call %s", bridge.session.ID)

	buffer := make([]byte, 1024) // 1KB buffer for audio data

	for {
		select {
		case <-bridge.stopChan:
			log.Printf("Stopping SIP-to-Agent audio streaming for call %s", bridge.session.ID)
			return
		default: // Read audio data from SIP caller
			n, err := bridge.audioReader.Read(buffer)
			if err != nil {
				if err.Error() != "EOF" && err != bridge.session.Context.Err() {
					log.Printf("Error reading audio from SIP for call %s: %v", bridge.session.ID, err)
				}
				return
			}

			if n > 0 {
				// Send the audio to the agent via the agent manager
				if err := bridge.sendAudioToAgent(buffer[:n]); err != nil {
					log.Printf("Error sending audio to agent %s: %v", bridge.agentExtension, err)
				}
			}
		}
	}
}

// sendAudioToAgent sends audio data from SIP caller to the assigned agent
func (bridge *AudioBridge) sendAudioToAgent(audioData []byte) error {
	if agentManager == nil {
		log.Printf("Agent manager not set, cannot send audio to agent %s", bridge.agentExtension)
		return nil
	}

	// Send the audio to the agent via the agent manager
	return agentManager.SendAudioToAgent(bridge.agentExtension, bridge.session.ID, audioData)
}

// GetActiveBridges returns the count of active bridges (for monitoring)
func GetActiveBridges() int {
	bridgesMutex.RLock()
	defer bridgesMutex.RUnlock()
	return len(activeBridges)
}
