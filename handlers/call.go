package handlers

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/Reverse-Call-Center/virtual-call-center/agents"
	"github.com/Reverse-Call-Center/virtual-call-center/audio"
	"github.com/Reverse-Call-Center/virtual-call-center/config"
	"github.com/Reverse-Call-Center/virtual-call-center/types"
)

var (
	ivrConfig   map[int]*config.Ivr
	queueConfig map[int]*config.Queue
)

func InitializeConfigs() {
	ivrConfig = make(map[int]*config.Ivr)
	queueConfig = make(map[int]*config.Queue)

	ivrConfigData, err := config.LoadIvrConfig()
	if err != nil {
		fmt.Printf("Error loading IVR config: %v\n", err)
		return
	}

	for _, ivr := range ivrConfigData.IVRs {
		if ivr.OptionId == 0 {
			fmt.Printf("Skipping IVR with OptionId 0: %v\n", ivr)
			continue
		}
		if _, exists := ivrConfig[ivr.OptionId]; exists {
			fmt.Printf("Duplicate IVR OptionId %d found, skipping: %v\n", ivr.OptionId, ivr)
			continue
		}
		fmt.Printf("Loading IVR: %v\n", ivr)
		ivrConfig[ivr.OptionId] = ivr
	}

	queueConfigData, err := config.LoadQueueConfig()
	if err != nil {
		fmt.Printf("Error loading Queue config: %v\n", err)
		return
	}
	for _, queue := range queueConfigData.Queues {
		if queue.OptionId == 0 {
			fmt.Printf("Skipping Queue with OptionId 0: %v\n", queue)
			continue
		}
		if _, exists := queueConfig[queue.OptionId]; exists {
			fmt.Printf("Duplicate Queue OptionId %d found, skipping: %v\n", queue.OptionId, queue)
			continue
		}
		fmt.Printf("Loading Queue: %v\n", queue)
		queueConfig[queue.OptionId] = queue
	}
}

func RouteCallToAction(session *types.CallSession, digit string) {
	currentIVR, exists := ivrConfig[session.IVRLevel]
	if !exists {
		fmt.Printf("Current IVR config not found for level %d\n", session.IVRLevel)
		return
	}

	var selectedOption *config.Option
	for _, option := range currentIVR.Options {
		if strconv.Itoa(option.OptionNumber) == digit {
			selectedOption = &option
			break
		}
	}

	if selectedOption == nil {
		fmt.Printf("Invalid option %s selected\n", digit)
		audio.PlayAudioFile(session, currentIVR.InvalidOptionMessage)
		HandleIVRFlow(session, currentIVR)
		return
	}

	action := selectedOption.OptionAction

	if action == 0 {
		fmt.Printf("Hanging up call %s\n", session.ID)
		session.Dialog.Hangup(session.Context)
		session.State = types.StateHangup
		return
	}

	for _, ivr := range ivrConfig {
		if ivr.OptionId == action {
			session.IVRLevel = action
			HandleIVRFlow(session, ivr)
			return
		}
	}

	for _, queue := range queueConfig {
		log.Printf("Checking queue %d for action %d\n", queue.OptionId, action)
		if queue.OptionId == action {
			HandleQueueLogic(session, queue)
			return
		}
	}

	fmt.Printf("No action found for action %d\n", action)
	audio.PlayAudioFile(session, currentIVR.InvalidOptionMessage)
	HandleIVRFlow(session, currentIVR)
}

func HandleIVRFlow(session *types.CallSession, ivrConfig *config.Ivr) {
	dtmfChan := make(chan string, 1)
	dtmfDone := make(chan struct{})
	audioDone := make(chan struct{})
	stopAudio := make(chan struct{})

	go func() {
		defer close(dtmfDone)
		audio.ListenForDTMF(session, dtmfChan)
	}()

	go func() {
		defer close(audioDone)
		if err := audio.PlayAudioFileInterruptible(session, ivrConfig.WelcomeMessage, stopAudio); err != nil {
			fmt.Printf("Error playing IVR welcome message for call %s: %v\n", session.ID, err)
			return
		}
	}()

	timeout := time.Duration(ivrConfig.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ivrTimer := time.NewTimer(timeout)
	defer ivrTimer.Stop()

	select {
	case digit := <-dtmfChan:
		close(stopAudio)
		if !ivrTimer.Stop() {
			<-ivrTimer.C
		}

		if digit == "" {
			fmt.Printf("Empty DTMF digit received for call %s\n", session.ID)
			return
		}

		fmt.Printf("Received DTMF digit: %s for call %s\n", digit, session.ID)
		RouteCallToAction(session, digit)
		return

	case <-audioDone:
		select {
		case digit := <-dtmfChan:
			if !ivrTimer.Stop() {
				<-ivrTimer.C
			}

			if digit == "" {
				fmt.Printf("Empty DTMF digit received after audio for call %s\n", session.ID)
				return
			}
			fmt.Printf("Received DTMF digit after audio: %s for call %s\n", digit, session.ID)
			RouteCallToAction(session, digit)
			return

		case <-ivrTimer.C:
			fmt.Printf("IVR timeout for call %s\n", session.ID)

			if ivrConfig.TimeoutMessage != "" {
				audio.PlayAudioFile(session, ivrConfig.TimeoutMessage)
			}

			if ivrConfig.TimeoutAction == 0 {
				fmt.Printf("Hanging up call %s due to timeout\n", session.ID)
				session.Dialog.Hangup(session.Context)
				session.State = types.StateHangup
			} else {
				RouteCallToAction(session, strconv.Itoa(ivrConfig.TimeoutAction))
			}
			return

		case <-session.Context.Done():
			fmt.Printf("Call %s context cancelled during IVR\n", session.ID)
			return

		case <-dtmfDone:
			fmt.Printf("DTMF listener ended for call %s\n", session.ID)
			return
		}

	case <-ivrTimer.C:
		close(stopAudio)
		fmt.Printf("IVR timeout for call %s\n", session.ID)

		if ivrConfig.TimeoutMessage != "" {
			audio.PlayAudioFile(session, ivrConfig.TimeoutMessage)
		}

		if ivrConfig.TimeoutAction == 0 {
			fmt.Printf("Hanging up call %s due to timeout\n", session.ID)
			session.Dialog.Hangup(session.Context)
			session.State = types.StateHangup
		} else {
			RouteCallToAction(session, strconv.Itoa(ivrConfig.TimeoutAction))
		}

	case <-session.Context.Done():
		close(stopAudio)
		fmt.Printf("Call %s context cancelled during IVR\n", session.ID)
		return

	case <-dtmfDone:
		close(stopAudio)
		fmt.Printf("DTMF listener ended for call %s\n", session.ID)
		return
	}
}

func HandleQueueLogic(session *types.CallSession, queueConfig *config.Queue) {
	fmt.Printf("Entering queue %d for call %s\n", queueConfig.OptionId, session.ID)
	session.State = types.StateQueue
	session.QueueID = queueConfig.OptionId

	agents.GetManager().AddToQueue(queueConfig.OptionId, session)

	queueTimer := time.NewTimer(time.Duration(queueConfig.Timeout) * time.Second)
	defer queueTimer.Stop()

	// Channel to stop hold music when call is assigned to agent
	stopHoldMusic := make(chan struct{})
	holdMusicDone := make(chan struct{})

	// Monitor for agent assignment to stop hold music
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-session.Context.Done():
				return
			case <-ticker.C:
				if session.State == types.StateWithAgent {
					fmt.Printf("Call %s assigned to agent %s, stopping hold music\n", session.ID, session.AgentExt)
					close(stopHoldMusic)
					return
				}
			}
		}
	}()
	go func() {
		defer close(holdMusicDone)
		lastAnnounceTime := time.Now()

		for {
			select {
			case <-session.Context.Done():
				fmt.Printf("Call %s context cancelled, stopping hold music\n", session.ID)
				return
			case <-stopHoldMusic:
				fmt.Printf("Hold music stopped for call %s (agent assigned)\n", session.ID)
				return
			default:
				// Double check state
				if session.State == types.StateConnected || session.State == types.StateWithAgent {
					fmt.Printf("Call %s state changed to %v, stopping hold music\n", session.ID, session.State)
					return
				}

				if time.Since(lastAnnounceTime) >= time.Duration(queueConfig.AnnounceTime)*time.Second {
					fmt.Printf("Playing announcement for call %s in queue %d\n", session.ID, queueConfig.OptionId)
					if err := audio.PlayAudioFileInterruptible(session, queueConfig.AnnounceMessage, stopHoldMusic); err != nil {
						fmt.Printf("Error playing announce message for call %s: %v\n", session.ID, err)
					}
					lastAnnounceTime = time.Now()

					// Check if we should stop after announcement
					select {
					case <-stopHoldMusic:
						fmt.Printf("Hold music stopped during announcement for call %s\n", session.ID)
						return
					default:
					}
				}

				// Use interruptible hold music that can be stopped immediately
				if err := audio.PlayAudioFileInterruptible(session, queueConfig.HoldMusic, stopHoldMusic); err != nil {
					if err != session.Context.Err() {
						fmt.Printf("Error playing hold music for call %s: %v\n", session.ID, err)
					}
					return
				}
			}
		}
	}()

	fmt.Printf("Call %s waiting in queue %d\n", session.ID, queueConfig.OptionId)

	select {
	case <-queueTimer.C:
		if session.State == types.StateQueue {
			fmt.Printf("Queue timeout for call %s\n", session.ID)
			session.State = types.StateConnected
			audio.PlayAudioFile(session, "ringing.wav")
		}

	case <-session.Context.Done():
		fmt.Printf("Call %s context cancelled while in queue\n", session.ID)
		return
	}

	// Wait for hold music to stop when agent is assigned
	select {
	case <-holdMusicDone:
		fmt.Printf("Hold music ended for call %s\n", session.ID)
	case <-session.Context.Done():
		fmt.Printf("Call %s context cancelled while waiting for hold music to end\n", session.ID)
		if session.AgentExt != "" {
			agents.GetManager().EndCall(session.AgentExt)
		}
		return
	}

	// Keep call active while with agent
	for session.State == types.StateConnected || session.State == types.StateWithAgent {
		select {
		case <-session.Context.Done():
			if session.AgentExt != "" {
				fmt.Printf("Call %s ended, cleaning up agent %s\n", session.ID, session.AgentExt)
				agents.GetManager().EndCall(session.AgentExt)
			}
			return
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func GetIVRConfig(optionId int) (*config.Ivr, bool) {
	ivr, exists := ivrConfig[optionId]
	return ivr, exists
}

func AssignCallToAgent(session *types.CallSession, agentExt string) {
	session.State = types.StateWithAgent
	session.AgentExt = agentExt
	fmt.Printf("Call %s assigned to agent %s\n", session.ID, agentExt)
}
