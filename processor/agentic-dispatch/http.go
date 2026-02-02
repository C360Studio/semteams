package agenticdispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/service"
	"github.com/google/uuid"
)

func init() {
	service.RegisterOpenAPISpec("agentic-dispatch", agenticDispatchOpenAPISpec())
}

// Compile-time check that Component implements the HTTP handler interface
var _ interface {
	RegisterHTTPHandlers(prefix string, mux *http.ServeMux)
} = (*Component)(nil)

// HTTPMessageRequest represents a message request via HTTP.
// This is the request format for the POST /message endpoint.
type HTTPMessageRequest struct {
	Content     string            `json:"content"`
	UserID      string            `json:"user_id,omitempty"`
	ChannelType string            `json:"channel_type,omitempty"`
	ChannelID   string            `json:"channel_id,omitempty"`
	ReplyTo     string            `json:"reply_to,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// HTTPMessageResponse represents the response from the HTTP message endpoint.
type HTTPMessageResponse struct {
	ResponseID string `json:"response_id"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	InReplyTo  string `json:"in_reply_to,omitempty"`
	Error      string `json:"error,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// RegisterHTTPHandlers registers HTTP endpoints for agentic-dispatch.
// This enables synchronous message processing via HTTP for web clients and E2E tests.
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	// POST /message - synchronous message processing
	mux.HandleFunc("POST "+prefix+"message", c.handleHTTPMessage)

	// GET /commands - list available commands
	mux.HandleFunc("GET "+prefix+"commands", c.handleListCommands)

	// GET /health - component health check
	mux.HandleFunc("GET "+prefix+"health", c.handleHTTPHealth)

	c.logger.Info("agentic-dispatch HTTP handlers registered", slog.String("prefix", prefix))
}

// handleHTTPMessage processes a user message synchronously via HTTP.
// Unlike the NATS path, this returns the response directly instead of publishing to a stream.
func (c *Component) handleHTTPMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startTime := time.Now()

	// Parse request body
	var req HTTPMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Content == "" {
		c.writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Apply defaults
	if req.UserID == "" {
		req.UserID = "http-user"
	}
	if req.ChannelType == "" {
		req.ChannelType = "http"
	}
	if req.ChannelID == "" {
		req.ChannelID = fmt.Sprintf("http-%d", time.Now().UnixNano())
	}

	// Build UserMessage
	msg := agentic.UserMessage{
		MessageID:   uuid.New().String(),
		ChannelType: req.ChannelType,
		ChannelID:   req.ChannelID,
		UserID:      req.UserID,
		Content:     req.Content,
		ReplyTo:     req.ReplyTo,
		Metadata:    req.Metadata,
		Timestamp:   time.Now(),
	}

	// Record message received metric
	c.metrics.recordMessageReceived(msg.ChannelType)

	c.logger.Debug("HTTP message received",
		slog.String("message_id", msg.MessageID),
		slog.String("user_id", msg.UserID),
		slog.String("channel", msg.ChannelType),
		slog.String("content_preview", truncate(msg.Content, 50)))

	// Process the message and get response synchronously
	resp := c.processMessageSync(ctx, msg)

	// Record routing duration
	duration := time.Since(startTime).Seconds()
	c.metrics.recordRoutingDuration(duration)

	// Convert to HTTP response format
	httpResp := HTTPMessageResponse{
		ResponseID: resp.ResponseID,
		Type:       resp.Type,
		Content:    resp.Content,
		InReplyTo:  resp.InReplyTo,
		Timestamp:  resp.Timestamp.Format(time.RFC3339),
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(httpResp); err != nil {
		c.logger.Error("Failed to encode HTTP response", slog.String("error", err.Error()))
	}
}

// processMessageSync processes a message and returns the response synchronously.
// This is used by the HTTP handler to avoid the pub/sub response path.
func (c *Component) processMessageSync(ctx context.Context, msg agentic.UserMessage) agentic.UserResponse {
	// Check if it's a command
	if strings.HasPrefix(msg.Content, "/") {
		return c.processCommandSync(ctx, msg)
	}

	// It's a task submission
	return c.processTaskSubmissionSync(ctx, msg)
}

// processCommandSync processes a command and returns the response synchronously.
func (c *Component) processCommandSync(ctx context.Context, msg agentic.UserMessage) agentic.UserResponse {
	name, cmd, args, found := c.registry.Match(msg.Content)
	if !found {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Unknown command. Type /help for available commands.",
			Timestamp:   time.Now(),
		}
	}

	// Check permission
	if cmd.Config.Permission != "" && !c.hasPermission(msg.UserID, cmd.Config.Permission) {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     fmt.Sprintf("Permission denied: requires '%s'", cmd.Config.Permission),
			Timestamp:   time.Now(),
		}
	}

	// Resolve loop ID
	loopID := ""
	if len(args) > 0 && args[0] != "" {
		loopID = args[0]
	} else if c.config.AutoContinue {
		loopID = c.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
	}

	// Check if loop is required
	if cmd.Config.RequireLoop && loopID == "" {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "No active loop. Specify a loop_id or start a task first.",
			Timestamp:   time.Now(),
		}
	}

	// Execute handler
	resp, err := cmd.Handler(ctx, msg, args, loopID)
	if err != nil {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     fmt.Sprintf("Command failed: %s", err.Error()),
			Timestamp:   time.Now(),
		}
	}

	// Record command executed
	c.metrics.recordCommandExecuted(name)

	c.logger.Info("HTTP command executed",
		slog.String("command", name),
		slog.String("user_id", msg.UserID))

	// Also publish to stream for async consumers (optional - allows CLI, other services to see responses)
	c.sendResponse(ctx, resp)

	return resp
}

// processTaskSubmissionSync processes a task submission and returns acknowledgment.
// The actual task execution happens asynchronously via NATS.
func (c *Component) processTaskSubmissionSync(ctx context.Context, msg agentic.UserMessage) agentic.UserResponse {
	// Check submit permission
	if !c.hasPermission(msg.UserID, "submit_task") {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Permission denied: cannot submit tasks",
			Timestamp:   time.Now(),
		}
	}

	// Determine loop ID (continue existing or create new)
	loopID := ""
	if msg.ReplyTo != "" {
		loopID = msg.ReplyTo
	} else if c.config.AutoContinue {
		loopID = c.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
	}

	// Create new loop if needed
	if loopID == "" {
		loopID = "loop_" + uuid.New().String()[:8]
	}

	taskID := uuid.New().String()

	// Create task message
	task := agentic.TaskMessage{
		LoopID: loopID,
		TaskID: taskID,
		Role:   c.config.DefaultRole,
		Model:  c.config.DefaultModel,
		Prompt: msg.Content,
	}

	// Track the loop
	c.loopTracker.Track(&LoopInfo{
		LoopID:        loopID,
		TaskID:        taskID,
		UserID:        msg.UserID,
		ChannelType:   msg.ChannelType,
		ChannelID:     msg.ChannelID,
		State:         "pending",
		MaxIterations: 20,
		CreatedAt:     time.Now(),
	})

	// Record loop started
	c.metrics.recordLoopStarted()

	// Publish task
	taskData, err := json.Marshal(task)
	if err != nil {
		c.logger.Error("Failed to marshal task", slog.String("error", err.Error()))
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Failed to create task. Please try again.",
			Timestamp:   time.Now(),
		}
	}

	subject := fmt.Sprintf("agent.task.%s", taskID)
	if err := c.natsClient.PublishToStream(ctx, subject, taskData); err != nil {
		c.logger.Error("Failed to publish task", slog.String("error", err.Error()))
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Failed to submit task. Please try again.",
			Timestamp:   time.Now(),
		}
	}

	// Record task submitted
	c.metrics.recordTaskSubmitted()

	c.logger.Info("HTTP task submitted",
		slog.String("loop_id", loopID),
		slog.String("task_id", taskID),
		slog.String("user_id", msg.UserID))

	// Create acknowledgment response
	resp := agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		InReplyTo:   loopID,
		Type:        agentic.ResponseTypeStatus,
		Content:     fmt.Sprintf("Task submitted. Loop: %s", loopID),
		Timestamp:   time.Now(),
	}

	// Also publish acknowledgment to stream
	c.sendResponse(ctx, resp)

	return resp
}

// handleListCommands returns the list of available commands.
func (c *Component) handleListCommands(w http.ResponseWriter, r *http.Request) {
	commands := c.registry.All()

	type commandInfo struct {
		Name    string `json:"name"`
		Help    string `json:"help"`
		Pattern string `json:"pattern"`
	}

	result := make([]commandInfo, 0, len(commands))
	for name, cfg := range commands {
		result = append(result, commandInfo{
			Name:    name,
			Help:    cfg.Help,
			Pattern: cfg.Pattern,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		c.logger.Error("Failed to encode commands list", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleHTTPHealth returns the component health status.
func (c *Component) handleHTTPHealth(w http.ResponseWriter, r *http.Request) {
	health := c.Health()

	w.Header().Set("Content-Type", "application/json")
	if !health.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(health); err != nil {
		c.logger.Error("Failed to encode health status", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeJSONError writes a JSON error response.
func (c *Component) writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := HTTPMessageResponse{
		ResponseID: uuid.New().String(),
		Type:       "error",
		Content:    message,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		c.logger.Error("Failed to encode error response", slog.String("error", err.Error()))
	}
}

// truncate truncates a string to the given length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// agenticDispatchOpenAPISpec returns the OpenAPI specification for agentic-dispatch endpoints.
func agenticDispatchOpenAPISpec() *service.OpenAPISpec {
	return &service.OpenAPISpec{
		Tags: []service.TagSpec{
			{
				Name:        "AgenticDispatch",
				Description: "User message processing and command dispatch",
			},
		},
		Paths: map[string]service.PathSpec{
			"/message": {
				POST: &service.OperationSpec{
					Summary:     "Process a user message",
					Description: "Processes a user message synchronously. Commands (starting with /) are executed immediately. Regular messages are submitted as tasks. Request body: {content: string (required), user_id?: string, channel_type?: string, channel_id?: string, reply_to?: string}",
					Tags:        []string{"AgenticDispatch"},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "Message processed successfully",
							ContentType: "application/json",
						},
						"400": {
							Description: "Invalid request",
						},
					},
				},
			},
			"/commands": {
				GET: &service.OperationSpec{
					Summary:     "List available commands",
					Description: "Returns the list of all registered commands with their descriptions and usage",
					Tags:        []string{"AgenticDispatch"},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "List of available commands",
							ContentType: "application/json",
						},
					},
				},
			},
			"/health": {
				GET: &service.OperationSpec{
					Summary:     "Component health check",
					Description: "Returns the health status of the agentic-dispatch component",
					Tags:        []string{"AgenticDispatch"},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "Component is healthy",
							ContentType: "application/json",
						},
						"503": {
							Description: "Component is unhealthy",
						},
					},
				},
			},
		},
	}
}
