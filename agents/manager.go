package agents

import (
	"fmt"
	"sync"
	"time"

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
	fmt.Printf("Agent %s (%s) registered\n", name, extension)
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

	am.queueCalls[queueID] = append(am.queueCalls[queueID], session)
}

func (am *AgentManager) PullFromQueue(extension string, queueID int) *types.CallSession {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	agent, exists := am.agents[extension]
	if !exists || agent.Status != pb.AgentStatus_AVAILABLE {
		return nil
	}

	calls, exists := am.queueCalls[queueID]
	if !exists || len(calls) == 0 {
		return nil
	}

	session := calls[0]
	am.queueCalls[queueID] = calls[1:]

	agent.CurrentCall = session
	agent.Status = pb.AgentStatus_BUSY
	session.State = types.StateWithAgent
	session.AgentExt = extension

	return session
}

func (am *AgentManager) EndCall(extension string) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if agent, exists := am.agents[extension]; exists {
		agent.CurrentCall = nil
		agent.Status = pb.AgentStatus_AVAILABLE
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

	return agent.Stream.Send(msg)
}
