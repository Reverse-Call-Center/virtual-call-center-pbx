package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/Reverse-Call-Center/virtual-call-center/types"
	"github.com/emiago/diago"
)

// PCMToWAV converts raw PCM data to WAV format
// Assumes 16-bit PCM, mono, 8000 Hz sample rate (standard for telephony)
func PCMToWAV(pcmData []byte) ([]byte, error) {
	var buf bytes.Buffer

	// WAV header
	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcmData))) // File size - 8
	buf.WriteString("WAVE")

	// Format chunk
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))    // Chunk size
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // Audio format (PCM)
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // Number of channels (mono)
	binary.Write(&buf, binary.LittleEndian, uint32(8000))  // Sample rate
	binary.Write(&buf, binary.LittleEndian, uint32(16000)) // Byte rate (SampleRate * NumChannels * BitsPerSample/8)
	binary.Write(&buf, binary.LittleEndian, uint16(2))     // Block align (NumChannels * BitsPerSample/8)
	binary.Write(&buf, binary.LittleEndian, uint16(16))    // Bits per sample

	// Data chunk
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(pcmData))) // Data size
	buf.Write(pcmData)

	return buf.Bytes(), nil
}

// PCMToWAVReader creates an io.Reader that streams PCM data as WAV format
func PCMToWAVReader(pcmData []byte) (io.Reader, error) {
	wavData, err := PCMToWAV(pcmData)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(wavData), nil
}

// StreamingPCMToWAVWriter wraps an io.Writer to convert streaming PCM to WAV
type StreamingPCMToWAVWriter struct {
	writer        io.Writer
	headerWritten bool
	totalBytes    int
}

func NewStreamingPCMToWAVWriter(writer io.Writer) *StreamingPCMToWAVWriter {
	return &StreamingPCMToWAVWriter{
		writer: writer,
	}
}

func (w *StreamingPCMToWAVWriter) Write(pcmData []byte) (int, error) {
	if !w.headerWritten {
		// Write WAV header (we'll assume a large data size for streaming)
		var headerBuf bytes.Buffer

		// RIFF header
		headerBuf.WriteString("RIFF")
		binary.Write(&headerBuf, binary.LittleEndian, uint32(0xFFFFFFFF-8)) // Max file size for streaming
		headerBuf.WriteString("WAVE")

		// Format chunk
		headerBuf.WriteString("fmt ")
		binary.Write(&headerBuf, binary.LittleEndian, uint32(16))    // Chunk size
		binary.Write(&headerBuf, binary.LittleEndian, uint16(1))     // Audio format (PCM)
		binary.Write(&headerBuf, binary.LittleEndian, uint16(1))     // Number of channels (mono)
		binary.Write(&headerBuf, binary.LittleEndian, uint32(8000))  // Sample rate
		binary.Write(&headerBuf, binary.LittleEndian, uint32(16000)) // Byte rate
		binary.Write(&headerBuf, binary.LittleEndian, uint16(2))     // Block align
		binary.Write(&headerBuf, binary.LittleEndian, uint16(16))    // Bits per sample

		// Data chunk header
		headerBuf.WriteString("data")
		binary.Write(&headerBuf, binary.LittleEndian, uint32(0xFFFFFFFF)) // Max data size for streaming

		_, err := w.writer.Write(headerBuf.Bytes())
		if err != nil {
			return 0, err
		}
		w.headerWritten = true
	}

	// Write the actual PCM data
	n, err := w.writer.Write(pcmData)
	w.totalBytes += n
	return n, err
}

// StreamingPCMPlayer handles streaming PCM audio to SIP callers using the playback interface
type StreamingPCMPlayer struct {
	session    *types.CallSession
	playback   diago.AudioPlayback
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	pcmBuffer  chan []byte
	stopChan   chan struct{}
	isActive   bool
	mutex      sync.RWMutex
}

// NewStreamingPCMPlayer creates a new streaming PCM player for a SIP session
func NewStreamingPCMPlayer(session *types.CallSession) (*StreamingPCMPlayer, error) {
	log.Printf("[PCM-DEBUG] Creating new streaming PCM player for call %s", session.ID)

	playback, err := session.Dialog.PlaybackCreate()
	if err != nil {
		log.Printf("[PCM-ERROR] Failed to create playback for call %s: %v", session.ID, err)
		return nil, fmt.Errorf("failed to create playback: %v", err)
	}
	log.Printf("[PCM-DEBUG] Successfully created playback interface for call %s", session.ID)

	pipeReader, pipeWriter := io.Pipe()
	log.Printf("[PCM-DEBUG] Created pipe for streaming PCM data for call %s", session.ID)

	player := &StreamingPCMPlayer{
		session:    session,
		playback:   playback,
		pipeReader: pipeReader,
		pipeWriter: pipeWriter,
		pcmBuffer:  make(chan []byte, 100), // Buffer for PCM chunks
		stopChan:   make(chan struct{}),
		isActive:   true,
	}

	log.Printf("[PCM-DEBUG] Starting streaming goroutines for call %s (buffer size: 100)", session.ID)
	// Start the streaming goroutines
	go player.streamProcessor()
	go player.playbackHandler()

	log.Printf("[PCM-DEBUG] StreamingPCMPlayer successfully created for call %s", session.ID)

	return player, nil
}

// streamProcessor converts PCM chunks to WAV format and writes to pipe
func (p *StreamingPCMPlayer) streamProcessor() {
	log.Printf("[PCM-DEBUG] Starting stream processor for call %s", p.session.ID)
	defer func() {
		log.Printf("[PCM-DEBUG] Stream processor stopping for call %s", p.session.ID)
		p.pipeWriter.Close()
	}()

	// Write WAV header once
	headerWritten := false
	chunksProcessed := 0

	for {
		select {
		case <-p.stopChan:
			log.Printf("[PCM-DEBUG] Stream processor received stop signal for call %s (processed %d chunks)",
				p.session.ID, chunksProcessed)
			return
		case pcmData := <-p.pcmBuffer:
			chunksProcessed++
			log.Printf("[PCM-DEBUG] Processing PCM chunk #%d (%d bytes) for call %s",
				chunksProcessed, len(pcmData), p.session.ID)

			if !headerWritten {
				// Write WAV header for streaming
				log.Printf("[PCM-DEBUG] Writing WAV header for call %s", p.session.ID)
				wavHeader := p.createWAVHeader()
				if _, err := p.pipeWriter.Write(wavHeader); err != nil {
					log.Printf("[PCM-ERROR] Error writing WAV header for call %s: %v", p.session.ID, err)
					return
				}
				headerWritten = true
				log.Printf("[PCM-DEBUG] WAV header written (%d bytes) for call %s", len(wavHeader), p.session.ID)
			}

			// Write PCM data directly (it will be part of the WAV stream)
			if _, err := p.pipeWriter.Write(pcmData); err != nil {
				log.Printf("[PCM-ERROR] Error writing PCM chunk #%d (%d bytes) for call %s: %v",
					chunksProcessed, len(pcmData), p.session.ID, err)
				return
			} else {
				log.Printf("[PCM-DEBUG] Successfully wrote PCM chunk #%d (%d bytes) for call %s",
					chunksProcessed, len(pcmData), p.session.ID)
			}
		}
	}
}

// playbackHandler manages the playback interface
func (p *StreamingPCMPlayer) playbackHandler() {
	log.Printf("[PCM-DEBUG] Starting playback handler for call %s", p.session.ID)
	// Start playing the WAV stream from the pipe
	_, err := p.playback.Play(p.pipeReader, "audio/wav")
	if err != nil {
		log.Printf("[PCM-ERROR] Playback error for call %s: %v", p.session.ID, err)
	} else {
		log.Printf("[PCM-DEBUG] Playback completed for call %s", p.session.ID)
	}
}

// WritePCM writes PCM data to the streaming player
func (p *StreamingPCMPlayer) WritePCM(pcmData []byte) error {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if !p.isActive {
		log.Printf("[PCM-ERROR] Attempted to write %d bytes to inactive streaming player for call %s",
			len(pcmData), p.session.ID)
		return fmt.Errorf("streaming player is not active")
	}

	log.Printf("[PCM-DEBUG] WritePCM called with %d bytes for call %s (buffer len: %d/%d)",
		len(pcmData), p.session.ID, len(p.pcmBuffer), cap(p.pcmBuffer))

	select {
	case p.pcmBuffer <- pcmData:
		log.Printf("[PCM-DEBUG] Successfully queued %d bytes in PCM buffer for call %s (buffer now: %d/%d)",
			len(pcmData), p.session.ID, len(p.pcmBuffer), cap(p.pcmBuffer))
		return nil
	default:
		// Buffer full, drop the audio chunk
		log.Printf("[PCM-WARNING] PCM buffer full (%d/%d), dropping audio chunk of %d bytes for call %s",
			len(p.pcmBuffer), cap(p.pcmBuffer), len(pcmData), p.session.ID)
		return nil
	}
}

// Stop stops the streaming player
func (p *StreamingPCMPlayer) Stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.isActive {
		log.Printf("[PCM-DEBUG] Stopping streaming PCM player for call %s (buffer len: %d)",
			p.session.ID, len(p.pcmBuffer))
		p.isActive = false
		close(p.stopChan)
		p.pipeWriter.Close()
		log.Printf("[PCM-DEBUG] Streaming PCM player stopped for call %s", p.session.ID)
	} else {
		log.Printf("[PCM-DEBUG] Streaming PCM player already stopped for call %s", p.session.ID)
	}
}

// createWAVHeader creates a WAV header for streaming audio
func (p *StreamingPCMPlayer) createWAVHeader() []byte {
	var buf bytes.Buffer

	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(0xFFFFFFFF-8)) // Max file size for streaming
	buf.WriteString("WAVE")

	// Format chunk
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))    // Chunk size
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // Audio format (PCM)
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // Number of channels (mono)
	binary.Write(&buf, binary.LittleEndian, uint32(8000))  // Sample rate
	binary.Write(&buf, binary.LittleEndian, uint32(16000)) // Byte rate
	binary.Write(&buf, binary.LittleEndian, uint16(2))     // Block align
	binary.Write(&buf, binary.LittleEndian, uint16(16))    // Bits per sample

	// Data chunk header
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(0xFFFFFFFF)) // Max data size for streaming

	return buf.Bytes()
}

// Simple PCM player that sends raw PCM data directly
type SimplePCMPlayer struct {
	session  *types.CallSession
	playback diago.AudioPlayback
	isActive bool
	mutex    sync.RWMutex
}

// NewSimplePCMPlayer creates a simple PCM player that sends raw PCM
func NewSimplePCMPlayer(session *types.CallSession) (*SimplePCMPlayer, error) {
	playback, err := session.Dialog.PlaybackCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to create playback: %v", err)
	}

	return &SimplePCMPlayer{
		session:  session,
		playback: playback,
		isActive: true,
	}, nil
}

// PlayPCMChunk plays a chunk of raw PCM data
func (p *SimplePCMPlayer) PlayPCMChunk(pcmData []byte) error {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if !p.isActive {
		return fmt.Errorf("player not active")
	}

	// Create reader from raw PCM data (no WAV conversion)
	reader := bytes.NewReader(pcmData)

	// Try playing as raw PCM
	_, err := p.playback.Play(reader, "audio/pcm")
	if err != nil {
		// If PCM doesn't work, try with basic audio format
		reader = bytes.NewReader(pcmData)
		_, err = p.playback.Play(reader, "audio/basic")
		if err != nil {
			log.Printf("Failed to play both PCM and basic audio: %v", err)
			return err
		}
	}

	return nil
}

// Stop stops the PCM player
func (p *SimplePCMPlayer) Stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.isActive = false
}
