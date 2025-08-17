# Agent Feature

The agent feature enables two-way communication between call center agents and the system using gRPC bidirectional streaming.

## Features

- **Agent Registration**: Agents can register with their extension and name
- **Call Queue Management**: Agents can pull calls from specific queues
- **Bidirectional Audio**: PCM audio streaming between agents and callers
- **Agent Status Management**: Track agent availability (Available, Busy, Away)
- **Automatic Call Assignment**: System assigns calls to available agents

## Agent States

- `AVAILABLE`: Ready to take calls
- `BUSY`: Currently on a call
- `AWAY`: Temporarily unavailable

## gRPC Communication

### Agent → Server Messages

1. **RegisterAgent**: Register with extension and name
2. **AudioData**: Send PCM audio data for active call
3. **PullFromQueue**: Request to pull next call from specified queue
4. **AgentStatusUpdate**: Update agent status

### Server → Agent Messages

1. **CallAssignment**: New call assigned to agent
2. **AudioData**: PCM audio from caller
3. **CallEnded**: Notification that call has ended

## API Contract

### gRPC Service Definition

```protobuf
service AgentService {
    rpc Connect(stream AgentMessage) returns (stream ServerMessage);
}
```

### Message Types

#### AgentMessage (Client → Server)

```protobuf
message AgentMessage {
    oneof message {
        RegisterAgent register = 1;
        AudioData audio = 2;
        PullFromQueue pull_queue = 3;
        AgentStatusUpdate status = 4;
    }
}
```

**RegisterAgent**
```protobuf
message RegisterAgent {
    string extension = 1;  // Required: Agent extension (e.g., "1001")
    string name = 2;       // Required: Agent display name
}
```

**AudioData** 
```protobuf
message AudioData {
    bytes pcm_data = 1;    // Required: PCM audio data (16-bit, 8kHz, mono)
    string call_id = 2;    // Required: Call ID for audio routing
}
```

**PullFromQueue**
```protobuf
message PullFromQueue {
    int32 queue_id = 1;    // Required: Queue ID to pull call from
}
```

**AgentStatusUpdate**
```protobuf
message AgentStatusUpdate {
    AgentStatus status = 1;  // Required: New agent status
}

enum AgentStatus {
    AVAILABLE = 0;
    BUSY = 1;
    AWAY = 2;
}
```

#### ServerMessage (Server → Client)

```protobuf
message ServerMessage {
    oneof message {
        CallAssignment call_assignment = 1;
        AudioData audio = 2;
        CallEnded call_ended = 3;
    }
}
```

**CallAssignment**
```protobuf
message CallAssignment {
    string call_id = 1;    // Unique identifier for the call
    string caller_id = 2;  // Caller's phone number
    int32 queue_id = 3;    // Queue ID where call originated
}
```

**AudioData**
```protobuf
message AudioData {
    bytes pcm_data = 1;    // PCM audio data from caller
    string call_id = 2;    // Call ID for audio routing
}
```

**CallEnded**
```protobuf
message CallEnded {
    string call_id = 1;    // Call ID that has ended
}
```

### Connection Flow

1. **Client establishes bidirectional stream**
   - Connect to gRPC endpoint
   - Create bidirectional stream to AgentService.Connect

2. **Client sends RegisterAgent message**
   - Required fields: extension, name
   - Server response: None (registration is implicit)

3. **Client can send any of these messages:**
   - `PullFromQueue`: Request next available call from specified queue
   - `AudioData`: Send PCM audio for active call
   - `AgentStatusUpdate`: Update availability status

4. **Server sends messages:**
   - `CallAssignment`: When call is assigned to agent
   - `AudioData`: PCM audio from caller during active call  
   - `CallEnded`: When call terminates

### Error Handling

- **Stream errors**: Implement reconnection logic
- **Invalid messages**: Server silently ignores malformed messages
- **Authentication**: Currently none (extension-based identification)
- **Concurrency**: One connection per agent extension

### State Management

**Agent States:**
- `AVAILABLE` (0): Ready to receive calls
- `BUSY` (1): Currently handling a call (auto-set on call assignment)
- `AWAY` (2): Temporarily unavailable

**Call States:**
- Agent pulls call → Call assigned → Audio streaming begins
- Call ends → Agent returns to AVAILABLE status
- Agent disconnects → All active calls terminated

### Audio Format Specification

- **Format**: 16-bit signed PCM
- **Sample Rate**: 8000 Hz
- **Channels**: Mono (1)
- **Endianness**: Little-endian
- **Frame Size**: 160-1600 bytes (20-200ms chunks recommended)
- **Encoding**: Raw PCM samples, no headers

### Integration Requirements

**Prerequisites:**
- gRPC client library for your chosen language
- Protobuf compiler (for generating client stubs)
- Audio processing capability for PCM format

**Network:**
- Server endpoint: `localhost:50051` (default)
- Protocol: HTTP/2 (gRPC)
- Connection: Persistent bidirectional stream

**Protobuf Definition:**
Available at: `proto/agent.proto`
Generate client stubs for your language using protoc
