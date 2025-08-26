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
			if agent != nil && agent.CurrentCall != nil {
				if err := s.handleAudioFromAgent(agent, m.Audio); err != nil {
					log.Printf("Error handling audio from agent %s: %v", extension, err)
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
		return fmt.Errorf("no active call")
	}

	log.Printf("Received %d bytes of PCM audio from agent %s for call %s",
		len(audioData.PcmData), agent.Extension, audioData.CallId)

	// Send PCM data to SIP caller via audio bridge
	return audio.SendPCMToSIP(audioData.CallId, audioData.PcmData)
}
