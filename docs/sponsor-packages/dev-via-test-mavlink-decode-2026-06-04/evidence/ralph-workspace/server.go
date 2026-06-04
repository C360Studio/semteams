package main

import (
	"encoding/json"
	"net/http"
)

// Server handles HTTP requests for heartbeat data.
type Server struct {
	receiver *Receiver
}

// NewServer creates a new HTTP server.
func NewServer(receiver *Receiver) *Server {
	return &Server{
		receiver: receiver,
	}
}

// HeartbeatResponse represents the JSON response for the /heartbeat endpoint.
type HeartbeatResponse struct {
	SystemID      uint8  `json:"system_id"`
	ComponentID   uint8  `json:"component_id"`
	AutopilotType uint8  `json:"autopilot_type"`
	BaseMode      uint8  `json:"base_mode"`
	ReceivedAt    string `json:"received_at"`
}

// ServeHeartbeat handles GET /heartbeat requests.
func (s *Server) ServeHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hb := s.receiver.GetHeartbeat()
	if hb == nil {
		http.Error(w, "no heartbeat data available", http.StatusNoContent)
		return
	}

	response := HeartbeatResponse{
		SystemID:      hb.SystemID,
		ComponentID:   hb.ComponentID,
		AutopilotType: hb.AutopilotType,
		BaseMode:      hb.BaseMode,
		ReceivedAt:    hb.ReceivedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RegisterHandlers registers all HTTP handlers.
func (s *Server) RegisterHandlers() {
	http.HandleFunc("/heartbeat", s.ServeHeartbeat)
}
