# PRD: WebSocket Chat Server

## Overview
A lightweight, multi-room WebSocket chat server built in Go. Clients connect via WebSocket, choose a nickname, join rooms, and exchange real-time messages.

## Requirements

### Functional
- **F1.** Clients connect via WebSocket at `ws://host:port/ws`
- **F2.** Clients must submit a `join` message with `nickname` and `room` to participate
- **F3.** Multi-room support — messages are broadcast only within the same room
- **F4.** System messages announce join/leave events to all room members
- **F5.** Clients can list active rooms via a `rooms` command
- **F6.** Server tracks client nicknames and enforces uniqueness per room
- **F7.** Graceful shutdown closes all connections cleanly

### Non-Functional
- **N1.** Written in Go (no external dependencies beyond stdlib)
- **N2.** Concurrent client handling via goroutines
- **N3.** Typed JSON message protocol
- **N4.** Configurable listen address via env var or flag

## Message Protocol
All messages are JSON with a `type` field.

| Type | Direction | Fields | Description |
|------|-----------|--------|-------------|
| `join` | Client→Server | `nickname`, `room` | Join a room with a nickname |
| `chat` | Both | `nickname`, `room`, `text` | Chat message |
| `system` | Server→Client | `room`, `text` | System notification (join/leave) |
| `rooms` | Client→Server | — | Request list of active rooms |
| `room_list` | Server→Client | `rooms: string[]` | Response to `rooms` request |
| `error` | Server→Client | `text` | Error message |
| `leave` | Client→Server | `room` | Leave a room |

## File Structure
```
tests/ws-chat-server/
├── main.go        # Entry point: flag parsing, HTTP server, graceful shutdown
├── hub.go         # Hub: room registry, client registry, message routing
├── client.go      # Client: WebSocket read/write pumps, message dispatch
└── message.go     # Message types and JSON serialization
```

## Edge Cases
- Duplicate nickname in same room → error
- Message to unjoined room → error
- Client disconnects without leave → cleanup and notify room
- Empty text or nickname → error
- Max message size: 512 bytes

## Acceptance Criteria
1. Start server, connect two clients, join same room, send messages — both receive them
2. Client in different room does not receive messages from another room
3. Join/leave system messages appear in the correct room only
4. `rooms` command returns correct list
5. Duplicate nickname in same room returns error
6. SIGINT triggers graceful shutdown, closing all client connections
