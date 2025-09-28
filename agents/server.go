package agents

import (
	"fmt"
	"io"
	"log"

	"github.com/Reverse-Call-Center/virtual-call-center/audio"
	pb "github.com/Reverse-Call-Center/virtual-call-center/proto"
)

type Server struct {
	pb.UnimplementedAgentServiceServer
}

func NewAgentServer() *Server {
	return &Server{}
}

func (s *Server) Connect(stream pb.AgentService_ConnectServer) error {
	var agent *Agent
	var extension string

	log.Printf("New gRPC connection established")

	defer func() {
		log.Printf("gRPC connection closing for extension: %s", extension)
		if agent != nil {
			GetManager().UnregisterAgent(extension)
		}
	}()

	for {
		log.Printf("Waiting for message from client...")
		msg, err := stream.Recv()
		if err == io.EOF {
			log.Printf("Client closed connection (EOF)")
			return nil
		}
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			return err
		}

		log.Printf("Received message type: %T", msg.Message)

		switch m := msg.Message.(type) {
		case *pb.AgentMessage_Register:
			extension = m.Register.Extension
			log.Printf("Registering agent: Extension=%s, Name=%s", m.Register.Extension, m.Register.Name)
			agent = GetManager().RegisterAgent(extension, m.Register.Name, stream)
			log.Printf("Agent registration complete: %s", extension)
		case *pb.AgentMessage_Audio:
			log.Printf("[AGENT-AUDIO-DEBUG] Received audio message from agent %s", extension)
			if agent != nil && agent.CurrentCall != nil {
				log.Printf("[AGENT-AUDIO-DEBUG] Agent %s has active call %s, processing %d bytes of audio",
					extension, agent.CurrentCall.ID, len(m.Audio.PcmData))
				if err := s.handleAudioFromAgent(agent, m.Audio); err != nil {
					log.Printf("[AGENT-AUDIO-ERROR] Error handling audio from agent %s: %v", extension, err)
				} else {
					log.Printf("[AGENT-AUDIO-DEBUG] Successfully processed audio from agent %s", extension)
				}
			} else {
				if agent == nil {
					log.Printf("[AGENT-AUDIO-ERROR] Cannot process audio: agent %s not registered", extension)
				} else {
					log.Printf("[AGENT-AUDIO-ERROR] Cannot process audio: agent %s has no active call", extension)
				}
			}

		case *pb.AgentMessage_PullQueue:
			log.Printf("Received PullQueue message from extension=%s, agent=%v", extension, agent != nil)
			if agent != nil {
				log.Printf("Agent %s attempting to pull from queue %d", extension, m.PullQueue.QueueId)
				session := GetManager().PullFromQueue(extension, int(m.PullQueue.QueueId))
				if session != nil {
					log.Printf("Successfully pulled call %s for agent %s from queue %d", session.ID, extension, m.PullQueue.QueueId)
					if err := GetManager().AssignCallToAgent(extension, session, int(m.PullQueue.QueueId)); err != nil {
						log.Printf("Error assigning call to agent %s: %v", extension, err)
					}
				} else {
					log.Printf("No call available for agent %s in queue %d", extension, m.PullQueue.QueueId)
				}
			} else {
				log.Printf("Agent not registered, cannot pull from queue %d (extension='%s')", m.PullQueue.QueueId, extension)
			}

		case *pb.AgentMessage_Status:
			if agent != nil {
				agent.mutex.Lock()
				agent.Status = m.Status.Status
				agent.mutex.Unlock()
			}
		}
	}
}

func (s *Server) handleAudioFromAgent(agent *Agent, audioData *pb.AudioData) error {
	if agent.CurrentCall == nil {
		log.Printf("[AGENT-AUDIO-ERROR] Agent %s has no active call for audio processing", agent.Extension)
		return fmt.Errorf("no active call")
	}

	log.Printf("[AGENT-AUDIO-DEBUG] Agent->SIP: Processing %d bytes of PCM audio from agent %s for call %s (claimed callId: %s)",
		len(audioData.PcmData), agent.Extension, agent.CurrentCall.ID, audioData.CallId)

	// Log PCM characteristics for debugging
	if len(audioData.PcmData) >= 8 {
		log.Printf("[AGENT-AUDIO-DEBUG] Agent->SIP PCM details - First 8 bytes: %v, Last 8 bytes: %v",
			audioData.PcmData[:8], audioData.PcmData[len(audioData.PcmData)-8:])
	} else if len(audioData.PcmData) > 0 {
		log.Printf("[AGENT-AUDIO-DEBUG] Agent->SIP short PCM packet: %v", audioData.PcmData)
	}

	// First try the call ID from the agent's message
	log.Printf("[AGENT-AUDIO-DEBUG] Agent->SIP: Attempting to send audio using call ID from message: %s", audioData.CallId)
	err := audio.SendPCMToSIP(audioData.CallId, audioData.PcmData)
	if err != nil {
		// If that fails, try the agent's current call ID
		log.Printf("[AGENT-AUDIO-DEBUG] Agent->SIP: Call ID %s not found, trying agent's current call %s",
			audioData.CallId, agent.CurrentCall.ID)
		err = audio.SendPCMToSIP(agent.CurrentCall.ID, audioData.PcmData)
		if err != nil {
			log.Printf("[AGENT-AUDIO-ERROR] Agent->SIP: Failed to send audio for both call IDs - message ID: %s, current ID: %s, error: %v",
				audioData.CallId, agent.CurrentCall.ID, err)
		} else {
			log.Printf("[AGENT-AUDIO-DEBUG] Agent->SIP: Successfully sent %d bytes using agent's current call ID %s",
				len(audioData.PcmData), agent.CurrentCall.ID)
		}
	} else {
		log.Printf("[AGENT-AUDIO-DEBUG] Agent->SIP: Successfully sent %d bytes using message call ID %s",
			len(audioData.PcmData), audioData.CallId)
	}

	return err
}
