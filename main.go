package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Reverse-Call-Center/virtual-call-center/config"
	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

var (
	globalConfig *config.Config
	ivrConfig    map[int]*config.Ivr
	queueConfig  map[int]*config.Queue
	activeCalls  map[string]*CallSession
	callsMutex   sync.RWMutex
)

type CallSession struct {
	ID        string
	CallerID  string
	Dialog    *diago.DialogServerSession
	State     CallState
	IVRLevel  int
	QueueID   int
	StartTime time.Time
	Context   context.Context
	Cancel    context.CancelFunc
}

type CallState int

const (
	StateConnecting CallState = iota
	StateIVR
	StateQueue
	StateConnected
	StateHangup
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Println("Virtual Call Center Starting...")
	config, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	globalConfig = config
	activeCalls = make(map[string]*CallSession)

	// Initialize IVR and Queue configs from your config
	initializeConfigs()

	startSIPServer(ctx)
}

func initializeConfigs() {
	ivrConfig = make(map[int]*config.Ivr)
	queueConfig = make(map[int]*config.Queue)

	// Load IVR config and handle error
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

	// Load Queue config and handle error
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

func startSIPServer(ctx context.Context) {
	fmt.Println("Starting SIP server on:", globalConfig.SIPProtocol, globalConfig.SIPListenAddress, ":", globalConfig.SIPPort)

	transport := diago.Transport{
		Transport: globalConfig.SIPProtocol,
		BindHost:  globalConfig.SIPListenAddress,
		BindPort:  globalConfig.SIPPort,
	}

	ua, err := sipgo.NewUA()
	if err != nil {
		fmt.Printf("Error creating SIP User Agent: %v\n", err)
		return
	}

	dg := diago.NewDiago(ua, diago.WithTransport(transport))

	dg.Serve(ctx, func(inDialog *diago.DialogServerSession) {
		handleIncomingCall(ctx, inDialog)
	})
}

func handleIncomingCall(parentCtx context.Context, inDialog *diago.DialogServerSession) {
	// Create unique call session
	callCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	callerID := extractCallerPhone(inDialog.InviteRequest.Headers())

	callSession := &CallSession{
		ID:        generateCallID(),
		CallerID:  callerID,
		Dialog:    inDialog,
		State:     StateConnecting,
		IVRLevel:  globalConfig.InitialOptionId,
		StartTime: time.Now(),
		Context:   callCtx,
		Cancel:    cancel,
	}

	// Register call session
	callsMutex.Lock()
	activeCalls[callSession.ID] = callSession
	callsMutex.Unlock()

	// Clean up on exit
	defer func() {
		callsMutex.Lock()
		delete(activeCalls, callSession.ID)
		callsMutex.Unlock()
		if globalConfig.LogPhoneNumbers {
			fmt.Printf("Call %s from %s ended\n", callSession.ID, callSession.CallerID)
		} else {
			fmt.Printf("Call %s ended\n", callSession.ID)
		}

	}()

	if globalConfig.LogPhoneNumbers {
		fmt.Printf("New call %s from %s\n", callSession.ID, callSession.CallerID)
	} else {
		fmt.Printf("New call %s\n", callSession.ID)
	}

	// Answer the call
	inDialog.Trying()
	inDialog.Answer()

	// Check if there's a disclaimer message to play
	if globalConfig.RecordDisclaimerMessage != "" {
		if err := playAudioFile(callSession, globalConfig.RecordDisclaimerMessage); err != nil {
			fmt.Printf("Error playing disclaimer for call %s: %v\n", callSession.ID, err)
		}
	}

	// Start IVR flow with the correct IVR config
	callSession.State = StateIVR
	if initialIVR, exists := ivrConfig[globalConfig.InitialOptionId]; exists {
		handleIVRFlow(callSession, initialIVR)
	} else {
		fmt.Printf("No IVR config found for OptionId %d\n", globalConfig.InitialOptionId)
		callSession.Dialog.Hangup(callSession.Context)
	}
}

func routeCallToAction(session *CallSession, digit string) {
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
		playAudioFile(session, currentIVR.InvalidOptionMessage)
		handleIVRFlow(session, currentIVR)
		return
	}

	action := selectedOption.OptionAction

	if action == 0 {
		fmt.Printf("Hanging up call %s\n", session.ID)
		session.Dialog.Hangup(session.Context)
		session.State = StateHangup
		return
	}

	for _, ivr := range ivrConfig {
		if ivr.OptionId == action {
			session.IVRLevel = action
			handleIVRFlow(session, ivr)
			return
		}
	}

	for _, queue := range queueConfig {
		log.Printf("Checking queue %d for action %d\n", queue.OptionId, action)
		if queue.OptionId == action {
			handleQueueLogic(session, queue)
			return
		}
	}

	fmt.Printf("No action found for action %d\n", action)
	playAudioFile(session, currentIVR.InvalidOptionMessage)
	handleIVRFlow(session, currentIVR)
}

func handleIVRFlow(session *CallSession, ivrConfig *config.Ivr) {
	dtmfChan := make(chan string, 1)
	dtmfDone := make(chan struct{})
	audioDone := make(chan struct{})
	stopAudio := make(chan struct{})

	go func() {
		defer close(dtmfDone)
		listenForDTMF(session, dtmfChan)
	}()

	go func() {
		defer close(audioDone)
		if err := playAudioFileInterruptible(session, ivrConfig.WelcomeMessage, stopAudio); err != nil {
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
		routeCallToAction(session, digit)
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
			routeCallToAction(session, digit)
			return

		case <-ivrTimer.C:
			fmt.Printf("IVR timeout for call %s\n", session.ID)

			if ivrConfig.TimeoutMessage != "" {
				playAudioFile(session, ivrConfig.TimeoutMessage)
			}

			if ivrConfig.TimeoutAction == 0 {
				fmt.Printf("Hanging up call %s due to timeout\n", session.ID)
				session.Dialog.Hangup(session.Context)
				session.State = StateHangup
			} else {
				routeCallToAction(session, strconv.Itoa(ivrConfig.TimeoutAction))
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
			playAudioFile(session, ivrConfig.TimeoutMessage)
		}

		if ivrConfig.TimeoutAction == 0 {
			fmt.Printf("Hanging up call %s due to timeout\n", session.ID)
			session.Dialog.Hangup(session.Context)
			session.State = StateHangup
		} else {
			routeCallToAction(session, strconv.Itoa(ivrConfig.TimeoutAction))
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

func handleQueueLogic(session *CallSession, queueConfig *config.Queue) {
	fmt.Printf("Entering queue %d for call %s\n", queueConfig.OptionId, session.ID)
	session.State = StateQueue
	session.QueueID = queueConfig.OptionId

	queueTimer := time.NewTimer(time.Duration(queueConfig.Timeout) * time.Second)
	defer queueTimer.Stop()

	holdMusicDone := make(chan struct{})
	go func() {
		defer close(holdMusicDone)
		lastAnnounceTime := time.Now()

		for {
			select {
			case <-session.Context.Done():
				return
			case <-holdMusicDone:
				return
			default:
				if time.Since(lastAnnounceTime) >= time.Duration(queueConfig.AnnounceTime)*time.Second {
					fmt.Printf("Playing announcement for call %s in queue %d\n", session.ID, queueConfig.OptionId)
					if err := playAudioFile(session, queueConfig.AnnounceMessage); err != nil {
						fmt.Printf("Error playing announce message for call %s: %v\n", session.ID, err)
					}
					lastAnnounceTime = time.Now()
					time.Sleep(500 * time.Millisecond)
				}

				if err := playAudioFile(session, queueConfig.HoldMusic); err != nil {
					fmt.Printf("Error playing hold music for call %s: %v\n", session.ID, err)
					return
				}
			}
		}
	}()

	fmt.Printf("Call %s waiting in queue %d\n", session.ID, queueConfig.OptionId)

	select {
	case <-queueTimer.C:
		fmt.Printf("Agent available for call %s\n", session.ID)
		session.State = StateConnected
		playAudioFile(session, "ringing.wav")

	case <-session.Context.Done():
		fmt.Printf("Call %s context cancelled while in queue\n", session.ID)
		return
	}
}

func playAudioFile(session *CallSession, filename string) error {
	playFile, err := os.Open("./sounds/" + filename)
	if err != nil {
		return fmt.Errorf("error opening audio file %s: %v", filename, err)
	}
	defer playFile.Close()

	pb, err := session.Dialog.PlaybackCreate()
	if err != nil {
		return fmt.Errorf("error creating playback: %v", err)
	}

	_, err = pb.Play(playFile, "audio/wav")
	if err != nil {
		return fmt.Errorf("error playing audio: %v", err)
	}

	return nil
}

func playAudioFileInterruptible(session *CallSession, filename string, stopChan <-chan struct{}) error {
	playFile, err := os.Open("./sounds/" + filename)
	if err != nil {
		return fmt.Errorf("error opening audio file %s: %v", filename, err)
	}
	defer playFile.Close()

	pb, err := session.Dialog.PlaybackCreate()
	if err != nil {
		return fmt.Errorf("error creating playback: %v", err)
	}

	playDone := make(chan error, 1)
	go func() {
		_, err := pb.Play(playFile, "audio/wav")
		playDone <- err
	}()

	select {
	case err := <-playDone:
		return err
	case <-stopChan:
		return nil
	case <-session.Context.Done():
		return session.Context.Err()
	}
}

func listenForDTMF(session *CallSession, dtmfChan chan<- string) {
	reader := session.Dialog.AudioReaderDTMF()

	err := reader.Listen(func(dtmf rune) error {
		log.Printf("Received DTMF digit: %s for call %s", string(dtmf), session.ID)
		select {
		case dtmfChan <- string(dtmf):
			// Successfully sent DTMF digit
			return fmt.Errorf("dtmf received") // Return error to stop listening after first digit
		case <-session.Context.Done():
			return session.Context.Err()
		default:
			// Channel is full, ignore
			log.Printf("DTMF channel full for call %s, ignoring digit %s", session.ID, string(dtmf))
		}
		return nil
	}, 10*time.Second) // Reduced timeout for DTMF detection

	if err != nil && err != context.Canceled && err.Error() != "dtmf received" {
		log.Printf("DTMF listening error for call %s: %v", session.ID, err)
	}
}

func extractCallerPhone(headers []sip.Header) string {
	for _, header := range headers {
		if header.Name() == "From" {
			from := header.Value()
			if after, ok := strings.CutPrefix(from, "<sip:"); ok {
				trimmed := after
				parts := strings.Split(strings.TrimSuffix(trimmed, ">"), "@")
				return parts[0]
			}
		}
	}
	return "unknown"
}

func generateCallID() string {
	return fmt.Sprintf("call_%d", time.Now().UnixNano())
}

// Helper function to get active call count
func getActiveCallCount() int {
	callsMutex.RLock()
	defer callsMutex.RUnlock()
	return len(activeCalls)
}

// Helper function to get calls in specific state
func getCallsInState(state CallState) []*CallSession {
	callsMutex.RLock()
	defer callsMutex.RUnlock()

	var calls []*CallSession
	for _, call := range activeCalls {
		if call.State == state {
			calls = append(calls, call)
		}
	}
	return calls
}
