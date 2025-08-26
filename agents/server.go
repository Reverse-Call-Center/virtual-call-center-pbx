package agents

import (
	"fmt"
	"io"
	"log"

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

	defer func() {
		if agent != nil {
			GetManager().UnregisterAgent(extension)
		}
	}()

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch m := msg.Message.(type) {
		case *pb.AgentMessage_Register:
			extension = m.Register.Extension
			agent = GetManager().RegisterAgent(extension, m.Register.Name, stream)

		case *pb.AgentMessage_Audio:
			if agent != nil && agent.CurrentCall != nil {
				if err := s.handleAudioFromAgent(agent, m.Audio); err != nil {
					log.Printf("Error handling audio from agent %s: %v", extension, err)
				}
			}
		case *pb.AgentMessage_PullQueue:
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
				log.Printf("Agent not registered, cannot pull from queue %d", m.PullQueue.QueueId)
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

func (s *Server) handleAudioFromAgent(agent *Agent, audio *pb.AudioData) error {
	if agent.CurrentCall == nil {
		return fmt.Errorf("no active call")
	}

	// For now, just log the audio data received - you'll need to implement
	// proper PCM to WAV conversion or use the appropriate format
	log.Printf("Received %d bytes of PCM audio from agent %s for call %s",
		len(audio.PcmData), agent.Extension, audio.CallId)

	// TODO: Convert PCM data to appropriate format and play
	// This is a placeholder - you'll need to implement audio format conversion
	return nil
}
