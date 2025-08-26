package audio

import (
	"bytes"
	"encoding/binary"
	"io"
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
