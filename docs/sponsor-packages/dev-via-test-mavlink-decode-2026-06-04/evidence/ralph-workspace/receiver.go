package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// HeartbeatData holds the most recent HEARTBEAT frame data.
type HeartbeatData struct {
	SystemID    uint8
	ComponentID uint8
	AutopilotType uint8
	BaseMode    uint8
	ReceivedAt  time.Time
}

// Receiver listens for MAVLink v2 HEARTBEAT frames over UDP.
type Receiver struct {
	mu   sync.RWMutex
	data *HeartbeatData
}

// NewReceiver creates a new MAVLink receiver.
func NewReceiver() *Receiver {
	return &Receiver{
		data: nil,
	}
}

// ParseHeartbeat parses a raw MAVLink v2 frame and extracts HEARTBEAT data.
// The frame format for v2 is: 
// [0]=0xFD (frame start), [1]=payload_len, [2]=incompat_flags, [3]=compat_flags, 
// [4]=seq, [5]=system_id, [6]=component_id, [7-9]=message_id (LE, 3 bytes), [10+]=payload
func (r *Receiver) ParseHeartbeat(frame []byte) error {
	if len(frame) < 10 {
		return fmt.Errorf("frame too short: %d bytes", len(frame))
	}

	// Check frame start byte.
	if frame[0] != 0xFD {
		return fmt.Errorf("invalid frame start byte: 0x%02x", frame[0])
	}

	// Parse the message ID from bytes [7-9] (little-endian, 3 bytes).
	msgID := uint32(frame[7]) | (uint32(frame[8]) << 8) | (uint32(frame[9]) << 16)
	
	if msgID != 0 {
		return fmt.Errorf("not a HEARTBEAT message (ID: %d)", msgID)
	}

	systemID := frame[5]
	componentID := frame[6]
	
	// HEARTBEAT payload starts at byte 10 (after header).
	// Payload structure (9 bytes):
	// [0-3]: custom_mode (uint32 LE)
	// [4]: autopilot_type (uint8)
	// [5]: base_mode (uint8)
	// [6]: system_status (uint8)
	// [7]: mavlink_version (uint8)
	// [8]: spare
	if len(frame) < 10+9 {
		return fmt.Errorf("frame too short for HEARTBEAT payload: %d bytes", len(frame))
	}

	autopilotType := frame[10+4]
	baseMode := frame[10+5]

	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = &HeartbeatData{
		SystemID:      systemID,
		ComponentID:   componentID,
		AutopilotType: autopilotType,
		BaseMode:      baseMode,
		ReceivedAt:    time.Now(),
	}

	return nil
}

// GetHeartbeat returns the most recent heartbeat data.
func (r *Receiver) GetHeartbeat() *HeartbeatData {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.data == nil {
		return nil
	}
	// Return a copy to avoid race conditions.
	data := *r.data
	return &data
}

// ListenUDP starts listening for MAVLink frames on the specified port.
func (r *Receiver) ListenUDP(port int) error {
	addr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP("0.0.0.0"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP:%d: %w", port, err)
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		frame := buffer[:n]
		// Try to parse as MAVLink v2 frame.
		if frame[0] == 0xFD {
			_ = r.ParseHeartbeat(frame)
		}
	}
}
