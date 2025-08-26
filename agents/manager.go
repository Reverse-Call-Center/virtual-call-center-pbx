package agents

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Reverse-Call-Center/virtual-call-center/audio"
	pb "github.com/Reverse-Call-Center/virtual-call-center/proto"
	"github.com/Reverse-Call-Center/virtual-call-center/types"
)

type Agent struct {
	Extension    string
	Name         string
	Status       pb.AgentStatus
	CurrentCall  *types.CallSession
	Stream       pb.AgentService_ConnectServer
	LastActivity time.Time
	mutex        sync.RWMutex
}

type AgentManager struct {
	agents     map[string]*Agent
	queueCalls map[int][]*types.CallSession
	mutex      sync.RWMutex
}

var manager *AgentManager

func init() {
	manager = &AgentManager{
		agents:     make(map[string]*Agent),
		queueCalls: make(map[int][]*types.CallSession),
	}
}

func GetManager() *AgentManager {
	return manager
}

func (am *AgentManager) RegisterAgent(extension, name string, stream pb.AgentService_ConnectServer) *Agent {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	agent := &Agent{
		Extension:    extension,
		Name:         name,
		Status:       pb.AgentStatus_AVAILABLE,
		Stream:       stream,
		LastActivity: time.Now(),
	}
	am.agents[extension] = agent
	fmt.Printf("Agent %s (%s) registered with status %v\n", name, extension, agent.Status)
	fmt.Printf("DEBUG: Total agents registered: %d\n", len(am.agents))
	return agent
}

func (am *AgentManager) UnregisterAgent(extension string) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if agent, exists := am.agents[extension]; exists {
		if agent.CurrentCall != nil {
			agent.CurrentCall.Cancel()
		}
		delete(am.agents, extension)
		fmt.Printf("Agent %s unregistered\n", extension)
	}
}

func (am *AgentManager) AddToQueue(queueID int, session *types.CallSession) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	fmt.Printf("DEBUG: Adding call %s to queue %d\n", session.ID, queueID)
	am.queueCalls[queueID] = append(am.queueCalls[queueID], session)
	fmt.Printf("DEBUG: Queue %d now has %d calls\n", queueID, len(am.queueCalls[queueID]))
}

func (am *AgentManager) PullFromQueue(extension string, queueID int) *types.CallSession {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	fmt.Printf("DEBUG: PullFromQueue - Extension: %s, QueueID: %d\n", extension, queueID)

	agent, exists := am.agents[extension]
	if !exists {
		fmt.Printf("DEBUG: Agent %s not found\n", extension)
		return nil
	}

	if agent.Status != pb.AgentStatus_AVAILABLE {
		fmt.Printf("DEBUG: Agent %s status is %v (not AVAILABLE)\n", extension, agent.Status)
		return nil
	}

	calls, exists := am.queueCalls[queueID]
	if !exists {
		fmt.Printf("DEBUG: Queue %d does not exist\n", queueID)
		return nil
	}

	if len(calls) == 0 {
		fmt.Printf("DEBUG: Queue %d has no calls (length: %d)\n", queueID, len(calls))
		return nil
	}

	fmt.Printf("DEBUG: Found %d calls in queue %d\n", len(calls), queueID)
	session := calls[0]
	am.queueCalls[queueID] = calls[1:]

	agent.CurrentCall = session
	agent.Status = pb.AgentStatus_BUSY
	session.State = types.StateWithAgent
	session.AgentExt = extension

	fmt.Printf("DEBUG: Successfully assigned call %s to agent %s\n", session.ID, extension)
	return session
}

func (am *AgentManager) EndCall(extension string) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if agent, exists := am.agents[extension]; exists {
		if agent.CurrentCall != nil {
			// Stop audio bridge when ending call
			audio.StopAudioBridge(agent.CurrentCall.ID)
		}
		agent.CurrentCall = nil
		agent.Status = pb.AgentStatus_AVAILABLE
		fmt.Printf("Call ended for agent %s, status set to AVAILABLE\n", extension)
	}
}

func (am *AgentManager) GetAvailableAgents() []*Agent {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	var available []*Agent
	for _, agent := range am.agents {
		if agent.Status == pb.AgentStatus_AVAILABLE {
			available = append(available, agent)
		}
	}
	return available
}

func (am *AgentManager) SendAudioToAgent(extension string, callID string, audioData []byte) error {
	am.mutex.RLock()
	agent, exists := am.agents[extension]
	am.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s not found", extension)
	}

	msg := &pb.ServerMessage{
		Message: &pb.ServerMessage_Audio{
			Audio: &pb.AudioData{
				PcmData: audioData,
				CallId:  callID,
			},
		},
	}
	return agent.Stream.Send(msg)
}

func (am *AgentManager) AssignCallToAgent(extension string, session *types.CallSession, queueID int) error {
	am.mutex.RLock()
	agent, exists := am.agents[extension]
	am.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s not found", extension)
	}
	msg := &pb.ServerMessage{
		Message: &pb.ServerMessage_CallAssignment{
			CallAssignment: &pb.CallAssignment{
				CallId:   session.ID,
				CallerId: session.CallerID,
				QueueId:  int32(queueID),
			},
		},
	}

	log.Printf("Sending call assignment to agent %s: CallId=%s, CallerId=%s, QueueId=%d",
		extension, session.ID, session.CallerID, queueID)

	err := agent.Stream.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send call assignment to agent: %v", err)
	}
	// Start audio bridge for bidirectional audio
	if err := audio.StartAudioBridge(session, extension); err != nil {
		fmt.Printf("Warning: Failed to start audio bridge for call %s with agent %s: %v\n", session.ID, extension, err)
		// Don't fail the call assignment due to audio bridge failure
	}

	return nil
}
