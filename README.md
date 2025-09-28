# Virtual Call Center PBX

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache-green.svg)](LICENSE)

A fully automated call center system designed for scambaiting operations. Keeps unwanted callers trapped in sophisticated hold-time scenarios while allowing scambaiters to act as fake agents.

## 🚀 Features

- **SIP Protocol Support**: Inbound-only SIP/VoIP with UDP/TCP transport
- **Interactive Voice Response (IVR)**: Multi-level menu systems with DTMF support
- **Call Queuing**: Queue management with hold music and announcements
- **Agent System**: gRPC-based agent communication with real-time audio streaming
- **Audio Processing**: PCM audio support with DTMF interruption
- **Configuration**: JSON-based configuration files for easy customization

## 📋 TODO

- [ ] Annoyance Modules (Periodic Are you a Human? / Are you still there)
- [ ] Call recording
- [ ] Database integration for call logs
- [ ] Advanced analytics and reporting
- [ ] Drop-In Updates

### Known Issues
- [ ] SIP compatibility improvements needed

## 📦 Installation

### Quick Start

```bash
# Clone repository
git clone https://github.com/Reverse-Call-Center/virtual-call-center.git
cd virtual-call-center

# Install dependencies
go mod tidy

# Generate protobuf files
protoc --go_out=. --go-grpc_out=. proto/agent.proto

# Build and run
go build -o virtual-call-center.exe .
./virtual-call-center.exe
```

### Configuration

Copy example config files and customize:
```bash
cp configs/config.json.example configs/config.json
cp configs/ivr.json.example configs/ivr.json
cp configs/queue.json.example configs/queue.json
```

Add your audio files to the `sounds/` directory (WAV format, 16-bit, 8kHz, mono).

## 🔧 Configuration Files

**Main Config (`configs/config.json`):**
- `sip_port`: SIP server port (default: 5080)
- `sip_host`: Bind address (0.0.0.0 for all interfaces)
- `initial_option_id`: Starting IVR menu ID
- `log_phone_numbers`: Enable/disable caller ID logging

**IVR Config (`configs/ivr.json`):**
- Define menu structures and DTMF key mappings
- Set timeout and retry behaviors
- Configure audio prompts for each menu option

**Queue Config (`configs/queue.json`):**
- Configure hold music and announcements
- Set queue timeouts and agent assignment rules

## 🔌 Agent Integration

The system provides a gRPC API for external agent applications:

1. **Connect** to gRPC server on port 50051
2. **Register** your agent with extension and name
3. **Receive** call assignments with caller information
4. **Handle** bidirectional audio streaming (PCM format)
5. **Update** agent status (Available, Busy, Away)

See `proto/agent.proto` for the complete API definition. Generate client code for your preferred language using protoc.

## 🎵 Audio Setup

Place WAV files in the `sounds/` directory:
- `cisco.wav` - Hold music
- `announce.wav` - Queue announcements  
- `disclaimer.wav` - Recording disclaimer
- `ivr_1000.wav` - Main menu prompt
- `goodbye.wav` - Call termination message

**Requirements**: WAV format, 16-bit, 8kHz, mono

## 📄 License

Apache License - see [LICENSE](LICENSE) file.

## ⚠️ Disclaimer

For legitimate scambaiting operations only. Users responsible for compliance with local laws regarding call recording and telecommunications.

---

**Built with a hate for scammers 💘**
