package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// componentSchema defines the configuration schema.
var componentSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Ensure Component implements required interfaces.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Component implements the A2A adapter input component.
// It receives A2A task requests and publishes them to NATS for agent processing.
type Component struct {
	name       string
	config     Config
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Task mapping
	taskMapper *TaskMapper

	// Agent card generation
	cardGenerator *AgentCardGenerator
	agentCard     *AgentCard
	cardMu        sync.RWMutex

	// HTTP server (for http transport)
	httpServer *http.Server

	// Lifecycle management
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// Context for background operations
	ctx    context.Context
	cancel context.CancelFunc

	// Metrics tracking
	tasksReceived  int64
	tasksCompleted int64
	tasksFailed    int64
	errors         int64
	lastActivity   time.Time
}

// NewComponent creates a new A2A adapter component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "unmarshal config")
	}

	// Use default config if ports not set
	if config.Ports == nil {
		config = DefaultConfig()
		// Re-unmarshal to get user-provided values
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.WrapInvalid(err, "Component", "NewComponent", "unmarshal config")
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "Component", "NewComponent", "validate config")
	}

	return &Component{
		name:       "a2a-adapter",
		config:     config,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		taskMapper: NewTaskMapper(),
	}, nil
}

// Initialize prepares the component.
func (c *Component) Initialize() error {
	// Create agent card generator
	c.cardGenerator = NewAgentCardGenerator(
		"http://"+c.config.ListenAddress,
		"", // Provider org can be set via config
	)

	return nil
}

// Start begins processing A2A requests.
func (c *Component) Start(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Component", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "Component", "Start", "context already cancelled")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return errs.WrapFatal(errs.ErrAlreadyStarted, "Component", "Start", "check running state")
	}

	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "check NATS client")
	}

	// Create cancellable context for background operations
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Start based on transport type
	switch c.config.Transport {
	case "http", "":
		if err := c.startHTTPServer(); err != nil {
			c.cancel()
			return errs.Wrap(err, "Component", "Start", "start HTTP server")
		}
	case "slim":
		// SLIM transport would subscribe to SLIM messages
		// This is a placeholder for future SLIM integration
		c.logger.Info("A2A adapter using SLIM transport",
			slog.String("group_id", c.config.SLIMGroupID))
	}

	// Start response listener
	go c.listenForResponses(c.ctx)

	c.running = true
	c.startTime = time.Now()

	c.logger.Info("A2A adapter started",
		slog.String("transport", c.config.Transport),
		slog.String("address", c.config.ListenAddress))

	return nil
}

// startHTTPServer starts the HTTP server for A2A requests.
func (c *Component) startHTTPServer() error {
	mux := http.NewServeMux()

	// Agent card endpoint
	mux.HandleFunc(c.config.AgentCardPath, c.handleAgentCard)

	// A2A task endpoints
	mux.HandleFunc("/tasks/send", c.handleSendTask)
	mux.HandleFunc("/tasks/get", c.handleGetTask)
	mux.HandleFunc("/tasks/cancel", c.handleCancelTask)

	c.httpServer = &http.Server{
		Addr:              c.config.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Use a listener to verify the server can bind to the address
	ln, err := net.Listen("tcp", c.config.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", c.config.ListenAddress, err)
	}

	// Start server with the listener
	go func() {
		if err := c.httpServer.Serve(ln); err != http.ErrServerClosed {
			c.logger.Error("HTTP server error", slog.Any("error", err))
		}
	}()

	return nil
}

// handleAgentCard serves the agent card.
func (c *Component) handleAgentCard(w http.ResponseWriter, _ *http.Request) {
	c.cardMu.RLock()
	card := c.agentCard
	c.cardMu.RUnlock()

	if card == nil {
		// Generate a minimal card if none cached
		card = &AgentCard{
			Name:        "SemStreams Agent",
			Description: "A2A-compatible agent powered by SemStreams",
			URL:         "http://" + c.config.ListenAddress,
			Version:     "1.0",
			Capabilities: []Capability{
				{Name: "task-execution", Description: "Execute delegated tasks"},
			},
			DefaultInputModes:  []string{"text"},
			DefaultOutputModes: []string{"text"},
		}
	}

	data, err := SerializeAgentCard(card)
	if err != nil {
		http.Error(w, "Failed to serialize agent card", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// handleSendTask handles incoming task requests.
func (c *Component) handleSendTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Enforce authentication if required
	if c.config.EnableAuthentication {
		if !c.isAuthenticated(r) {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
	}

	// Limit request body size to prevent DoS (1MB max)
	const maxBodySize = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	// Parse request body
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "Invalid task format", http.StatusBadRequest)
		c.incrementErrors()
		return
	}

	// Extract requester DID from authentication
	requesterDID := c.extractRequesterDID(r)

	// Convert to TaskMessage
	taskMsg, err := c.taskMapper.ToTaskMessage(&task, requesterDID)
	if err != nil {
		http.Error(w, "Failed to process task: "+err.Error(), http.StatusBadRequest)
		c.incrementErrors()
		return
	}

	// Publish to NATS
	data, err := json.Marshal(taskMsg)
	if err != nil {
		http.Error(w, "Failed to serialize task", http.StatusInternalServerError)
		c.incrementErrors()
		return
	}

	subject := "agent.task.a2a." + sanitizeSubject(task.ID)
	if err := c.natsClient.PublishToStream(c.ctx, subject, data); err != nil {
		http.Error(w, "Failed to publish task", http.StatusInternalServerError)
		c.incrementErrors()
		return
	}

	c.mu.Lock()
	c.tasksReceived++
	c.lastActivity = time.Now()
	c.mu.Unlock()

	// Return initial status
	response := c.taskMapper.CreateTaskStatusUpdate(task.ID, "submitted", "Task accepted for processing")
	responseData, _ := json.Marshal(response)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(responseData)

	c.logger.Debug("A2A task received",
		slog.String("task_id", task.ID),
		slog.String("requester", requesterDID))
}

// handleGetTask handles task status queries.
func (c *Component) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		http.Error(w, "Task ID required", http.StatusBadRequest)
		return
	}

	// TODO: Look up task status from storage
	// For now, return a placeholder
	response := c.taskMapper.CreateTaskStatusUpdate(taskID, "working", "Task is being processed")
	responseData, _ := json.Marshal(response)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(responseData)
}

// handleCancelTask handles task cancellation requests.
func (c *Component) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Enforce authentication if required
	if c.config.EnableAuthentication {
		if !c.isAuthenticated(r) {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
	}

	// Limit request body size to prevent DoS (1MB max)
	const maxBodySize = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var request struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// TODO: Implement task cancellation via NATS
	// For now, acknowledge the request
	response := c.taskMapper.CreateTaskStatusUpdate(request.ID, "canceled", "Task cancellation requested")
	responseData, _ := json.Marshal(response)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(responseData)

	c.logger.Debug("A2A task cancellation requested", slog.String("task_id", request.ID))
}

// extractRequesterDID extracts the requester's DID from authentication headers.
func (c *Component) extractRequesterDID(r *http.Request) string {
	// Check for DID in Authorization header
	auth := r.Header.Get("Authorization")
	if auth != "" {
		// Parse DID from header (simplified)
		// In production, this would validate DID signatures
		return auth
	}

	// Check for X-Agent-DID header
	if did := r.Header.Get("X-Agent-DID"); did != "" {
		return did
	}

	return "anonymous"
}

// isAuthenticated checks if the request has valid authentication.
func (c *Component) isAuthenticated(r *http.Request) bool {
	// Check for Authorization header
	auth := r.Header.Get("Authorization")
	if auth != "" {
		// In production, this would validate DID signatures or tokens
		return true
	}

	// Check for X-Agent-DID header
	if did := r.Header.Get("X-Agent-DID"); did != "" {
		return true
	}

	return false
}

// listenForResponses subscribes to task completion events.
func (c *Component) listenForResponses(ctx context.Context) {
	// This would subscribe to agent.complete.* subjects and
	// send A2A responses back to requesters
	c.logger.Debug("A2A response listener started")

	<-ctx.Done()
}

// incrementErrors safely increments the error counter.
func (c *Component) incrementErrors() {
	c.mu.Lock()
	c.errors++
	c.mu.Unlock()
}

// UpdateAgentCard updates the cached agent card.
func (c *Component) UpdateAgentCard(card *AgentCard) {
	c.cardMu.Lock()
	c.agentCard = card
	c.cardMu.Unlock()
}

// Stop gracefully stops the component.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	// Cancel background context
	if c.cancel != nil {
		c.cancel()
	}

	// Stop HTTP server
	if c.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := c.httpServer.Shutdown(ctx); err != nil {
			c.logger.Warn("Failed to shutdown HTTP server", slog.Any("error", err))
		}
	}

	c.running = false
	c.logger.Info("A2A adapter stopped")

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "a2a-adapter",
		Type:        "input",
		Description: "Receives A2A task requests and publishes to NATS",
		Version:     "1.0.0",
	}
}

// InputPorts returns configured input port definitions.
func (c *Component) InputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Inputs))
	for i, portDef := range c.config.Ports.Inputs {
		ports[i] = component.Port{
			Name:        portDef.Name,
			Direction:   component.DirectionInput,
			Required:    portDef.Required,
			Description: portDef.Description,
			Config: component.NATSPort{
				Subject: portDef.Subject,
			},
		}
	}
	return ports
}

// OutputPorts returns configured output port definitions.
func (c *Component) OutputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		port := component.Port{
			Name:        portDef.Name,
			Direction:   component.DirectionOutput,
			Required:    portDef.Required,
			Description: portDef.Description,
		}
		if portDef.Type == "jetstream" {
			port.Config = component.JetStreamPort{
				StreamName: portDef.StreamName,
				Subjects:   []string{portDef.Subject},
			}
		} else {
			port.Config = component.NATSPort{
				Subject: portDef.Subject,
			}
		}
		ports[i] = port
	}
	return ports
}

// ConfigSchema returns the configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return componentSchema
}

// Health returns the current health status.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := "stopped"
	if c.running {
		status = "running"
	}

	return component.HealthStatus{
		Healthy:    c.running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errors),
		Uptime:     time.Since(c.startTime),
		Status:     status,
	}
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var errorRate float64
	total := c.tasksReceived + c.errors
	if total > 0 {
		errorRate = float64(c.errors) / float64(total)
	}

	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         errorRate,
		LastActivity:      c.lastActivity,
	}
}

// sanitizeSubject converts a task ID to a valid NATS subject component.
func sanitizeSubject(taskID string) string {
	result := make([]byte, len(taskID))
	for i := 0; i < len(taskID); i++ {
		if taskID[i] == '.' || taskID[i] == ':' || taskID[i] == '/' {
			result[i] = '-'
		} else {
			result[i] = taskID[i]
		}
	}
	return string(result)
}
