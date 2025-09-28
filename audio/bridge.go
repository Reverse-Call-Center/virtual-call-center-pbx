package audio

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

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

// checksumBytes calculates a simple checksum for debugging purposes
func checksumBytes(data []byte) uint32 {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	return sum
}

// StartAudioBridge starts bidirectional audio bridging between agent and caller
func StartAudioBridge(session *types.CallSession, agentExtension string) error {
	bridgesMutex.Lock()
	defer bridgesMutex.Unlock()

	log.Printf("[AUDIO-DEBUG] Starting audio bridge for call %s with agent %s", session.ID, agentExtension)

	// Check if bridge already exists
	if _, exists := activeBridges[session.ID]; exists {
		log.Printf("[AUDIO-DEBUG] Bridge already exists for call %s, returning error", session.ID)
		return fmt.Errorf("audio bridge already exists for call %s", session.ID)
	}

	log.Printf("[AUDIO-DEBUG] Creating new audio bridge for call %s, total bridges before: %d", session.ID, len(activeBridges))

	// Get audio reader from dialog session
	log.Printf("[AUDIO-DEBUG] Getting audio reader from dialog session for call %s", session.ID)
	audioReader, err := session.Dialog.AudioReader()
	if err != nil {
		log.Printf("[AUDIO-ERROR] Failed to get audio reader for call %s: %v", session.ID, err)
		return fmt.Errorf("failed to get audio reader for call %s: %v", session.ID, err)
	}
	log.Printf("[AUDIO-DEBUG] Successfully obtained audio reader for call %s", session.ID)

	// Create the PCM player for streaming audio to SIP caller
	log.Printf("[AUDIO-DEBUG] Creating PCM player for streaming Agent->SIP audio for call %s", session.ID)
	pcmPlayer, err := NewStreamingPCMPlayer(session)
	if err != nil {
		log.Printf("[AUDIO-ERROR] Failed to create PCM player for call %s: %v", session.ID, err)
		return fmt.Errorf("failed to create PCM player for call %s: %v", session.ID, err)
	}
	log.Printf("[AUDIO-DEBUG] Successfully created PCM player for call %s", session.ID)

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
	log.Printf("[AUDIO-DEBUG] Starting SIP->Agent audio streaming goroutine for call %s", session.ID)
	go bridge.streamSIPToAgent()

	log.Printf("[AUDIO-DEBUG] Audio bridge fully initialized for call %s with PCM player streaming (Agent->SIP direction ready)", session.ID)
	return nil
}

// StopAudioBridge stops the audio bridge for a call
func StopAudioBridge(callID string) {
	bridgesMutex.Lock()
	defer bridgesMutex.Unlock()

	log.Printf("[AUDIO-DEBUG] Stopping audio bridge for call %s, total bridges before: %d", callID, len(activeBridges))

	bridge, exists := activeBridges[callID]
	if !exists {
		log.Printf("[AUDIO-DEBUG] No audio bridge found for call %s to stop", callID)
		return
	}

	bridge.mutex.Lock()
	if bridge.isActive {
		log.Printf("[AUDIO-DEBUG] Deactivating bridge for call %s (was active)", callID)
		bridge.isActive = false
		close(bridge.stopChan)

		// Stop the PCM player
		if bridge.pcmPlayer != nil {
			log.Printf("[AUDIO-DEBUG] Stopping PCM player for call %s", callID)
			bridge.pcmPlayer.Stop()
		} else {
			log.Printf("[AUDIO-DEBUG] No PCM player to stop for call %s", callID)
		}

		log.Printf("[AUDIO-DEBUG] Signaled stop for audio bridge %s", callID)
	} else {
		log.Printf("[AUDIO-DEBUG] Audio bridge for call %s was already inactive", callID)
	}
	bridge.mutex.Unlock()

	delete(activeBridges, callID)
	log.Printf("[AUDIO-DEBUG] Audio bridge stopped for call %s, remaining bridges: %d", callID, len(activeBridges))
}

// SendPCMToSIP converts PCM audio from agent and sends it to SIP caller
func SendPCMToSIP(callID string, pcmData []byte) error {
	bridgesMutex.RLock()
	bridge, exists := activeBridges[callID]
	bridgesMutex.RUnlock()
	if !exists {
		log.Printf("[AUDIO-ERROR] No audio bridge found for call %s (Agent->SIP direction)", callID)
		return fmt.Errorf("no audio bridge found for call %s", callID)
	}

	bridge.mutex.RLock()
	defer bridge.mutex.RUnlock()
	if !bridge.isActive {
		log.Printf("[AUDIO-ERROR] Audio bridge not active for call %s (Agent->SIP direction)", callID)
		return fmt.Errorf("audio bridge not active for call %s", callID)
	}

	log.Printf("[AUDIO-DEBUG] Agent->SIP: Received %d bytes of PCM audio from agent %s for call %s",
		len(pcmData), bridge.agentExtension, callID)

	// Log PCM data characteristics for debugging
	if len(pcmData) > 0 {
		log.Printf("[AUDIO-DEBUG] Agent->SIP PCM details - First 8 bytes: %v, Last 8 bytes: %v",
			pcmData[:min(8, len(pcmData))], pcmData[max(0, len(pcmData)-8):])
	}

	// Stream PCM audio to SIP caller using audio bridge
	return bridge.streamPCMToSIP(pcmData)
}

// SendPCMToSIPByAgent finds active bridge by agent extension and sends PCM audio
func SendPCMToSIPByAgent(agentExtension string, pcmData []byte) error {
	bridgesMutex.RLock()
	defer bridgesMutex.RUnlock()

	log.Printf("[AUDIO-DEBUG] Agent->SIP: Looking for active bridge for agent %s to send %d bytes", agentExtension, len(pcmData))

	for callID, bridge := range activeBridges {
		if bridge.agentExtension == agentExtension && bridge.isActive {
			log.Printf("[AUDIO-DEBUG] Agent->SIP: Found active bridge for agent %s using call %s", agentExtension, callID)
			log.Printf("[AUDIO-DEBUG] Agent->SIP: Received %d bytes of PCM audio from agent %s for call %s",
				len(pcmData), bridge.agentExtension, callID)

			// Stream the PCM data to SIP
			return bridge.streamPCMToSIP(pcmData)
		}
	}

	log.Printf("[AUDIO-ERROR] No active audio bridge found for agent %s (checked %d bridges)", agentExtension, len(activeBridges))

	return fmt.Errorf("no active audio bridge found for agent %s", agentExtension)
}

// streamPCMToSIP streams PCM audio data to the SIP caller
func (bridge *AudioBridge) streamPCMToSIP(pcmData []byte) error {
	if bridge.pcmPlayer == nil {
		log.Printf("[AUDIO-ERROR] PCM player not initialized for call %s (Agent->SIP direction)", bridge.session.ID)
		return fmt.Errorf("PCM player not initialized for call %s", bridge.session.ID)
	}

	// Add comprehensive debugging info about the PCM data
	log.Printf("[AUDIO-DEBUG] Agent->SIP streamPCMToSIP: Processing %d bytes for call %s", len(pcmData), bridge.session.ID)
	if len(pcmData) >= 8 {
		log.Printf("[AUDIO-DEBUG] Agent->SIP PCM analysis - Length: %d bytes, First 8 bytes: %v, Checksum: %d",
			len(pcmData), pcmData[:8], checksumBytes(pcmData))
	} else {
		log.Printf("[AUDIO-DEBUG] Agent->SIP PCM analysis - Short packet: %d bytes, Data: %v",
			len(pcmData), pcmData)
	}

	// Write PCM data to the streaming player
	start := time.Now()
	err := bridge.pcmPlayer.WritePCM(pcmData)
	writeTime := time.Since(start)

	if err != nil {
		log.Printf("[AUDIO-ERROR] Agent->SIP: Failed to write %d bytes of PCM data to player for call %s (took %v): %v",
			len(pcmData), bridge.session.ID, writeTime, err)
		return err
	}

	log.Printf("[AUDIO-DEBUG] Agent->SIP: Successfully queued %d bytes of PCM audio for SIP caller (call %s, write took %v)",
		len(pcmData), bridge.session.ID, writeTime)
	return nil
}

// streamSIPToAgent reads audio from SIP caller and sends it to agent
func (bridge *AudioBridge) streamSIPToAgent() {
	bridge.mutex.Lock()
	if !bridge.isActive {
		log.Printf("[AUDIO-DEBUG] SIP->Agent: Bridge already inactive for call %s, exiting", bridge.session.ID)
		bridge.mutex.Unlock()
		return
	}
	bridge.readerActive = true
	bridge.mutex.Unlock()

	log.Printf("[AUDIO-DEBUG] SIP->Agent: Started audio streaming for call %s (agent %s)", bridge.session.ID, bridge.agentExtension)

	buffer := make([]byte, 1024) // 1KB buffer for audio data
	bytesRead := 0
	packetsRead := 0
	startTime := time.Now()

	for {
		select {
		case <-bridge.stopChan:
			elapsed := time.Since(startTime)
			log.Printf("[AUDIO-DEBUG] SIP->Agent: Stopping streaming for call %s after %v (read %d packets, %d total bytes)",
				bridge.session.ID, elapsed, packetsRead, bytesRead)
			return
		default: // Read audio data from SIP caller
			readStart := time.Now()
			n, err := bridge.audioReader.Read(buffer)
			readTime := time.Since(readStart)

			if err != nil {
				if err.Error() != "EOF" && err != bridge.session.Context.Err() {
					log.Printf("[AUDIO-ERROR] SIP->Agent: Error reading audio from SIP for call %s after %v: %v",
						bridge.session.ID, time.Since(startTime), err)
				} else {
					log.Printf("[AUDIO-DEBUG] SIP->Agent: Audio stream ended for call %s (%s) after %v",
						bridge.session.ID, err.Error(), time.Since(startTime))
				}
				return
			}

			if n > 0 {
				packetsRead++
				bytesRead += n

				log.Printf("[AUDIO-DEBUG] SIP->Agent: Read %d bytes in %v (packet #%d, total: %d bytes) for call %s",
					n, readTime, packetsRead, bytesRead, bridge.session.ID)

				// Log audio data characteristics
				if n >= 8 {
					log.Printf("[AUDIO-DEBUG] SIP->Agent audio analysis - First 8 bytes: %v, Checksum: %d",
						buffer[:8], checksumBytes(buffer[:n]))
				}

				// Send the audio to the agent via the agent manager
				if err := bridge.sendAudioToAgent(buffer[:n]); err != nil {
					log.Printf("[AUDIO-ERROR] SIP->Agent: Failed to send %d bytes to agent %s: %v",
						n, bridge.agentExtension, err)
				} else {
					log.Printf("[AUDIO-DEBUG] SIP->Agent: Successfully sent %d bytes to agent %s",
						n, bridge.agentExtension)
				}
			}
		}
	}
}

// sendAudioToAgent sends audio data from SIP caller to the assigned agent
func (bridge *AudioBridge) sendAudioToAgent(audioData []byte) error {
	if agentManager == nil {
		log.Printf("[AUDIO-ERROR] SIP->Agent: Agent manager not set, cannot send %d bytes to agent %s",
			len(audioData), bridge.agentExtension)
		return nil
	}

	log.Printf("[AUDIO-DEBUG] SIP->Agent: Sending %d bytes via agent manager to agent %s for call %s",
		len(audioData), bridge.agentExtension, bridge.session.ID)

	// Send the audio to the agent via the agent manager
	return agentManager.SendAudioToAgent(bridge.agentExtension, bridge.session.ID, audioData)
}

// GetActiveBridges returns the count of active bridges (for monitoring)
func GetActiveBridges() int {
	bridgesMutex.RLock()
	defer bridgesMutex.RUnlock()
	return len(activeBridges)
}
