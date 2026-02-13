package directorybridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// MockDirectory provides a test mock for the AGNTCY directory service.
type MockDirectory struct {
	server *httptest.Server
	mu     sync.RWMutex

	// Stored registrations
	registrations map[string]*storedRegistration

	// Configurable behavior
	failNextRegister   bool
	failNextHeartbeat  bool
	failNextDeregister bool
	registerDelay      time.Duration

	// Call counters for assertions
	RegisterCalls   int
	HeartbeatCalls  int
	DeregisterCalls int
	DiscoverCalls   int
}

// storedRegistration represents a registration in the mock.
type storedRegistration struct {
	Request   *RegistrationRequest
	ExpiresAt time.Time
}

// NewMockDirectory creates a new mock directory server.
func NewMockDirectory() *MockDirectory {
	md := &MockDirectory{
		registrations: make(map[string]*storedRegistration),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/agents", md.handleRegister)
	mux.HandleFunc("POST /v1/agents/{id}/heartbeat", md.handleHeartbeat)
	mux.HandleFunc("DELETE /v1/agents/{id}", md.handleDeregister)
	mux.HandleFunc("POST /v1/agents/discover", md.handleDiscover)

	md.server = httptest.NewServer(mux)
	return md
}

// URL returns the mock server's URL.
func (md *MockDirectory) URL() string {
	return md.server.URL
}

// Close shuts down the mock server.
func (md *MockDirectory) Close() {
	md.server.Close()
}

// SetFailNextRegister makes the next register call fail.
func (md *MockDirectory) SetFailNextRegister(fail bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.failNextRegister = fail
}

// SetFailNextHeartbeat makes the next heartbeat call fail.
func (md *MockDirectory) SetFailNextHeartbeat(fail bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.failNextHeartbeat = fail
}

// SetFailNextDeregister makes the next deregister call fail.
func (md *MockDirectory) SetFailNextDeregister(fail bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.failNextDeregister = fail
}

// SetRegisterDelay adds a delay to register calls.
func (md *MockDirectory) SetRegisterDelay(d time.Duration) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.registerDelay = d
}

// GetRegistration returns a stored registration by ID.
func (md *MockDirectory) GetRegistration(id string) *RegistrationRequest {
	md.mu.RLock()
	defer md.mu.RUnlock()
	if reg, ok := md.registrations[id]; ok {
		return reg.Request
	}
	return nil
}

// RegistrationCount returns the number of active registrations.
func (md *MockDirectory) RegistrationCount() int {
	md.mu.RLock()
	defer md.mu.RUnlock()
	return len(md.registrations)
}

func (md *MockDirectory) handleRegister(w http.ResponseWriter, r *http.Request) {
	md.mu.Lock()
	md.RegisterCalls++

	if md.registerDelay > 0 {
		delay := md.registerDelay
		md.mu.Unlock()
		time.Sleep(delay)
		md.mu.Lock()
	}

	if md.failNextRegister {
		md.failNextRegister = false
		md.mu.Unlock()
		writeJSONError(w, http.StatusInternalServerError, "simulated registration failure")
		return
	}
	md.mu.Unlock()

	var req RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Generate registration ID
	regID := "reg-" + req.AgentDID
	expiresAt := time.Now().Add(time.Duration(req.TTL) * time.Second)
	if req.TTL == 0 {
		expiresAt = time.Now().Add(5 * time.Minute)
	}

	md.mu.Lock()
	md.registrations[regID] = &storedRegistration{
		Request:   &req,
		ExpiresAt: expiresAt,
	}
	md.mu.Unlock()

	resp := RegistrationResponse{
		Success:        true,
		RegistrationID: regID,
		ExpiresAt:      expiresAt,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (md *MockDirectory) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	md.mu.Lock()
	md.HeartbeatCalls++

	if md.failNextHeartbeat {
		md.failNextHeartbeat = false
		md.mu.Unlock()
		writeJSONError(w, http.StatusInternalServerError, "simulated heartbeat failure")
		return
	}
	md.mu.Unlock()

	regID := r.PathValue("id")

	md.mu.Lock()
	reg, ok := md.registrations[regID]
	if !ok {
		md.mu.Unlock()
		writeJSONError(w, http.StatusNotFound, "registration not found")
		return
	}

	// Extend expiration
	reg.ExpiresAt = time.Now().Add(5 * time.Minute)
	md.mu.Unlock()

	resp := HeartbeatResponse{
		Success:   true,
		ExpiresAt: reg.ExpiresAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (md *MockDirectory) handleDeregister(w http.ResponseWriter, r *http.Request) {
	md.mu.Lock()
	md.DeregisterCalls++

	if md.failNextDeregister {
		md.failNextDeregister = false
		md.mu.Unlock()
		writeJSONError(w, http.StatusInternalServerError, "simulated deregistration failure")
		return
	}

	regID := r.PathValue("id")
	delete(md.registrations, regID)
	md.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (md *MockDirectory) handleDiscover(w http.ResponseWriter, r *http.Request) {
	md.mu.Lock()
	md.DiscoverCalls++
	md.mu.Unlock()

	var query DiscoveryQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	md.mu.RLock()
	agents := make([]DiscoveredAgent, 0, len(md.registrations))
	for _, reg := range md.registrations {
		agent := DiscoveredAgent{
			AgentDID:     reg.Request.AgentDID,
			OASFRecord:   reg.Request.OASFRecord,
			RegisteredAt: time.Now().Add(-time.Hour), // Simulated
			ExpiresAt:    reg.ExpiresAt,
		}
		agents = append(agents, agent)

		if query.Limit > 0 && len(agents) >= query.Limit {
			break
		}
	}
	md.mu.RUnlock()

	resp := DiscoveryResponse{
		Agents: agents,
		Total:  len(agents),
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   message,
	})
}
