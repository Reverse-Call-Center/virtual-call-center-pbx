package audio

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Reverse-Call-Center/virtual-call-center/types"
)

func PlayAudioFile(session *types.CallSession, filename string) error {
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

func PlayAudioFileInterruptible(session *types.CallSession, filename string, stopChan <-chan struct{}) error {
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

func ListenForDTMF(session *types.CallSession, dtmfChan chan<- string) {
	reader := session.Dialog.AudioReaderDTMF()

	err := reader.Listen(func(dtmf rune) error {
		log.Printf("Received DTMF digit: %s for call %s", string(dtmf), session.ID)
		select {
		case dtmfChan <- string(dtmf):
			return fmt.Errorf("dtmf received")
		case <-session.Context.Done():
			return session.Context.Err()
		default:
			log.Printf("DTMF channel full for call %s, ignoring digit %s", session.ID, string(dtmf))
		}
		return nil
	}, 10*time.Second)

	if err != nil && err != session.Context.Err() && err.Error() != "dtmf received" {
		log.Printf("DTMF listening error for call %s: %v", session.ID, err)
	}
}
