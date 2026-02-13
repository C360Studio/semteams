package directorybridge

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semstreams/agentic/identity"
	oasfgenerator "github.com/c360studio/semstreams/processor/oasf-generator"
)

// RegistrationManager handles the lifecycle of agent registrations.
type RegistrationManager struct {
	client           *DirectoryClient
	identityProvider identity.Provider
	config           Config
	logger           *slog.Logger

	// Active registrations
	registrations map[string]*Registration // entityID -> registration
	mu            sync.RWMutex

	// Heartbeat management
	heartbeatStop chan struct{}
	heartbeatWg   sync.WaitGroup
	stopOnce      sync.Once
	started       bool
	startMu       sync.Mutex
}

// Registration represents an active directory registration.
type Registration struct {
	// EntityID is the SemStreams entity ID.
	EntityID string `json:"entity_id"`

	// RegistrationID is the directory's registration ID.
	RegistrationID string `json:"registration_id"`

	// AgentDID is the agent's decentralized identifier.
	AgentDID string `json:"agent_did"`

	// OASFRecord is the agent's OASF specification.
	OASFRecord *oasfgenerator.OASFRecord `json:"oasf_record"`

	// RegisteredAt is when the registration was created.
	RegisteredAt time.Time `json:"registered_at"`

	// ExpiresAt is when the registration expires.
	ExpiresAt time.Time `json:"expires_at"`

	// LastHeartbeat is when the last heartbeat was sent.
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// Retries is the number of registration retries.
	Retries int `json:"retries"`
}

// NewRegistrationManager creates a new registration manager.
func NewRegistrationManager(client *DirectoryClient, identityProvider identity.Provider, config Config, logger *slog.Logger) *RegistrationManager {
	return &RegistrationManager{
		client:           client,
		identityProvider: identityProvider,
		config:           config,
		logger:           logger,
		registrations:    make(map[string]*Registration),
	}
}

// Start begins the heartbeat goroutine.
func (rm *RegistrationManager) Start(ctx context.Context) error {
	rm.startMu.Lock()
	defer rm.startMu.Unlock()

	if rm.started {
		return nil // Already started
	}

	rm.heartbeatStop = make(chan struct{})
	rm.stopOnce = sync.Once{} // Reset for potential restart
	rm.heartbeatWg.Add(1)
	go rm.heartbeatLoop(ctx)
	rm.started = true
	return nil
}

// Stop stops the heartbeat goroutine and deregisters all agents.
func (rm *RegistrationManager) Stop(ctx context.Context) error {
	rm.startMu.Lock()
	if !rm.started {
		rm.startMu.Unlock()
		return nil
	}
	rm.started = false
	rm.startMu.Unlock()

	// Use sync.Once to safely close the channel exactly once
	rm.stopOnce.Do(func() {
		if rm.heartbeatStop != nil {
			close(rm.heartbeatStop)
		}
	})
	rm.heartbeatWg.Wait()

	// Deregister all agents
	rm.mu.RLock()
	registrations := make([]*Registration, 0, len(rm.registrations))
	for _, reg := range rm.registrations {
		registrations = append(registrations, reg)
	}
	rm.mu.RUnlock()

	for _, reg := range registrations {
		if err := rm.Deregister(ctx, reg.EntityID); err != nil {
			rm.logger.Warn("Failed to deregister agent on shutdown",
				slog.String("entity_id", reg.EntityID),
				slog.Any("error", err))
		}
	}

	return nil
}

// RegisterAgent registers an agent with the directory.
func (rm *RegistrationManager) RegisterAgent(ctx context.Context, entityID string, record *oasfgenerator.OASFRecord, agentIdentity *identity.AgentIdentity) error {
	// Get or create DID
	var agentDID string
	if agentIdentity != nil {
		agentDID = agentIdentity.DIDString()
	} else if rm.identityProvider != nil {
		// Create identity for the agent
		newIdentity, err := rm.identityProvider.CreateIdentity(ctx, identity.CreateIdentityOptions{
			DisplayName: record.Name,
		})
		if err != nil {
			return err
		}
		agentDID = newIdentity.DIDString()
	}

	// Create registration request
	req := &RegistrationRequest{
		AgentDID:   agentDID,
		OASFRecord: record,
		TTL:        int(rm.config.GetRegistrationTTL().Seconds()),
		Metadata: map[string]any{
			"semstreams_entity_id": entityID,
			"source":               "semstreams",
		},
	}

	// Register with directory
	resp, err := rm.client.Register(ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		return &RegistrationError{
			EntityID: entityID,
			Message:  resp.Error,
		}
	}

	// Store registration
	registration := &Registration{
		EntityID:       entityID,
		RegistrationID: resp.RegistrationID,
		AgentDID:       agentDID,
		OASFRecord:     record,
		RegisteredAt:   time.Now(),
		ExpiresAt:      resp.ExpiresAt,
		LastHeartbeat:  time.Now(),
	}

	rm.mu.Lock()
	rm.registrations[entityID] = registration
	rm.mu.Unlock()

	rm.logger.Info("Registered agent with directory",
		slog.String("entity_id", entityID),
		slog.String("registration_id", resp.RegistrationID),
		slog.String("agent_did", agentDID))

	return nil
}

// UpdateRegistration updates an existing registration with new OASF data.
func (rm *RegistrationManager) UpdateRegistration(ctx context.Context, entityID string, record *oasfgenerator.OASFRecord) error {
	rm.mu.RLock()
	existing, ok := rm.registrations[entityID]
	rm.mu.RUnlock()

	if !ok {
		// Not registered yet, register now
		return rm.RegisterAgent(ctx, entityID, record, nil)
	}

	// Re-register with updated OASF record
	req := &RegistrationRequest{
		AgentDID:   existing.AgentDID,
		OASFRecord: record,
		TTL:        int(rm.config.GetRegistrationTTL().Seconds()),
		Metadata: map[string]any{
			"semstreams_entity_id": entityID,
			"source":               "semstreams",
		},
	}

	resp, err := rm.client.Register(ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		return &RegistrationError{
			EntityID: entityID,
			Message:  resp.Error,
		}
	}

	// Update registration
	rm.mu.Lock()
	existing.OASFRecord = record
	existing.RegistrationID = resp.RegistrationID
	existing.ExpiresAt = resp.ExpiresAt
	rm.mu.Unlock()

	rm.logger.Info("Updated agent registration",
		slog.String("entity_id", entityID),
		slog.String("registration_id", resp.RegistrationID))

	return nil
}

// Deregister removes an agent from the directory.
func (rm *RegistrationManager) Deregister(ctx context.Context, entityID string) error {
	rm.mu.Lock()
	registration, ok := rm.registrations[entityID]
	if ok {
		delete(rm.registrations, entityID)
	}
	rm.mu.Unlock()

	if !ok {
		return nil // Not registered
	}

	err := rm.client.Deregister(ctx, &DeregistrationRequest{
		RegistrationID: registration.RegistrationID,
		AgentDID:       registration.AgentDID,
	})
	if err != nil {
		return err
	}

	rm.logger.Info("Deregistered agent from directory",
		slog.String("entity_id", entityID),
		slog.String("registration_id", registration.RegistrationID))

	return nil
}

// GetRegistration returns the registration for an entity.
func (rm *RegistrationManager) GetRegistration(entityID string) *Registration {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.registrations[entityID]
}

// ListRegistrations returns all active registrations.
func (rm *RegistrationManager) ListRegistrations() []*Registration {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*Registration, 0, len(rm.registrations))
	for _, reg := range rm.registrations {
		result = append(result, reg)
	}
	return result
}

// heartbeatLoop sends periodic heartbeats to maintain registrations.
func (rm *RegistrationManager) heartbeatLoop(ctx context.Context) {
	defer rm.heartbeatWg.Done()

	ticker := time.NewTicker(rm.config.GetHeartbeatInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.heartbeatStop:
			return
		case <-ticker.C:
			rm.sendHeartbeats(ctx)
		}
	}
}

// sendHeartbeats sends heartbeats for all registrations.
func (rm *RegistrationManager) sendHeartbeats(ctx context.Context) {
	rm.mu.RLock()
	registrations := make([]*Registration, 0, len(rm.registrations))
	for _, reg := range rm.registrations {
		registrations = append(registrations, reg)
	}
	rm.mu.RUnlock()

	for _, reg := range registrations {
		// Check if heartbeat is needed (expiry approaching)
		if time.Until(reg.ExpiresAt) > rm.config.GetHeartbeatInterval()*2 {
			continue
		}

		resp, err := rm.client.Heartbeat(ctx, &HeartbeatRequest{
			RegistrationID: reg.RegistrationID,
			AgentDID:       reg.AgentDID,
		})
		if err != nil {
			rm.logger.Warn("Heartbeat failed",
				slog.String("entity_id", reg.EntityID),
				slog.Any("error", err))
			continue
		}

		rm.mu.Lock()
		reg.LastHeartbeat = time.Now()
		reg.ExpiresAt = resp.ExpiresAt
		rm.mu.Unlock()

		rm.logger.Debug("Heartbeat sent",
			slog.String("entity_id", reg.EntityID),
			slog.Time("expires_at", resp.ExpiresAt))
	}
}

// RegistrationError represents a registration failure.
type RegistrationError struct {
	EntityID string
	Message  string
}

func (e *RegistrationError) Error() string {
	return "registration failed for " + e.EntityID + ": " + e.Message
}
