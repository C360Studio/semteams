package teamsdispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/google/uuid"
)

// registerBuiltinCommands registers all built-in commands
func (c *Component) registerBuiltinCommands() {
	// /cancel [loop_id] - Cancel a loop
	c.registry.Register("cancel", CommandConfig{
		Pattern:     `^/cancel\s*(\S*)$`,
		Permission:  "cancel_own",
		RequireLoop: false,
		Help:        "/cancel [loop_id] - Cancel current or specified loop",
	}, c.handleCancelCommand)

	// /status [loop_id] - Show loop status
	c.registry.Register("status", CommandConfig{
		Pattern:     `^/status\s*(\S*)$`,
		Permission:  "view",
		RequireLoop: false,
		Help:        "/status [loop_id] - Show loop status",
	}, c.handleStatusCommand)

	// /loops - List active loops
	c.registry.Register("loops", CommandConfig{
		Pattern:     `^/loops$`,
		Permission:  "view",
		RequireLoop: false,
		Help:        "/loops - List your active loops",
	}, c.handleLoopsCommand)

	// /help - Show help
	c.registry.Register("help", CommandConfig{
		Pattern:     `^/help$`,
		Permission:  "",
		RequireLoop: false,
		Help:        "/help - Show available commands",
	}, c.handleHelpCommand)

	// /approve [loop_id] [reason] - Approve pending result
	c.registry.Register("approve", CommandConfig{
		Pattern:     `^/approve\s*(\S*)\s*(.*)$`,
		Permission:  "approve",
		RequireLoop: false,
		Help:        "/approve [loop_id] [reason] - Approve pending result",
	}, c.makeSignalCommand(agentic.SignalApprove))

	// /reject [loop_id] [reason] - Reject pending result
	c.registry.Register("reject", CommandConfig{
		Pattern:     `^/reject\s*(\S*)\s*(.*)$`,
		Permission:  "approve",
		RequireLoop: false,
		Help:        "/reject [loop_id] [reason] - Reject pending result with optional reason",
	}, c.makeSignalCommand(agentic.SignalReject))

	// /pause [loop_id] - Pause loop at next checkpoint
	c.registry.Register("pause", CommandConfig{
		Pattern:     `^/pause\s*(\S*)$`,
		Permission:  "cancel_own",
		RequireLoop: false,
		Help:        "/pause [loop_id] - Pause current or specified loop",
	}, c.makeSignalCommand(agentic.SignalPause))

	// /resume [loop_id] - Resume paused loop
	c.registry.Register("resume", CommandConfig{
		Pattern:     `^/resume\s*(\S*)$`,
		Permission:  "cancel_own",
		RequireLoop: false,
		Help:        "/resume [loop_id] - Resume paused loop",
	}, c.makeSignalCommand(agentic.SignalResume))

	// /onboard - Start operating-model onboarding interview
	c.registry.Register("onboard", CommandConfig{
		Pattern:     `^/onboard$`,
		Permission:  "submit_task",
		RequireLoop: false,
		Help:        "/onboard - Start the operating-model onboarding interview",
	}, c.handleOnboardCommand)
}

// handleCancelCommand handles the /cancel command
func (c *Component) handleCancelCommand(ctx context.Context, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
	// Use provided loop ID or active loop
	targetLoopID := loopID
	if len(args) > 0 && args[0] != "" {
		targetLoopID = args[0]
	}

	if targetLoopID == "" {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "No loop to cancel. Specify a loop_id or have an active loop.",
			Timestamp:   time.Now(),
		}, nil
	}

	// Check permission to control this loop
	if !c.canUserControlLoop(msg.UserID, targetLoopID) {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     "Permission denied: cannot cancel this loop",
			Timestamp:   time.Now(),
		}, nil
	}

	// Check if loop exists and is not already terminal
	loopInfo := c.loopTracker.Get(targetLoopID)
	if loopInfo == nil {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     fmt.Sprintf("Loop %s not found", targetLoopID),
			Timestamp:   time.Now(),
		}, nil
	}

	if isTerminalState(loopInfo.State) {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeStatus,
			Content:     fmt.Sprintf("Loop %s already in terminal state: %s", targetLoopID, loopInfo.State),
			Timestamp:   time.Now(),
		}, nil
	}

	// Send cancel signal
	signal := agentic.UserSignal{
		SignalID:    uuid.New().String(),
		Type:        agentic.SignalCancel,
		LoopID:      targetLoopID,
		UserID:      msg.UserID,
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		Timestamp:   time.Now(),
	}

	signalData, err := json.Marshal(signal)
	if err != nil {
		return agentic.UserResponse{}, errs.Wrap(err, "Component", "handleCancelCommand", "marshal signal")
	}

	subject := fmt.Sprintf("agent.signal.%s", targetLoopID)
	if err := c.natsClient.Publish(ctx, subject, signalData); err != nil {
		return agentic.UserResponse{}, errs.WrapTransient(err, "Component", "handleCancelCommand", "publish signal")
	}

	return agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		InReplyTo:   targetLoopID,
		Type:        agentic.ResponseTypeStatus,
		Content:     fmt.Sprintf("Cancel signal sent to loop %s", targetLoopID),
		Timestamp:   time.Now(),
	}, nil
}

// handleStatusCommand handles the /status command
func (c *Component) handleStatusCommand(ctx context.Context, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
	// Use provided loop ID or active loop
	targetLoopID := loopID
	if len(args) > 0 && args[0] != "" {
		targetLoopID = args[0]
	}

	c.logger.DebugContext(ctx, "Status command executed",
		slog.String("target_loop", targetLoopID),
		slog.String("user_id", msg.UserID))

	if targetLoopID == "" {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeStatus,
			Content:     "No active loop. Start a task or specify a loop_id.",
			Timestamp:   time.Now(),
		}, nil
	}

	loopInfo := c.loopTracker.Get(targetLoopID)
	if loopInfo == nil {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeError,
			Content:     fmt.Sprintf("Loop %s not found", targetLoopID),
			Timestamp:   time.Now(),
		}, nil
	}

	age := time.Since(loopInfo.CreatedAt).Truncate(time.Second)
	content := fmt.Sprintf("Loop: %s\nState: %s\nIterations: %d/%d\nAge: %s\nUser: %s",
		loopInfo.LoopID,
		loopInfo.State,
		loopInfo.Iterations,
		loopInfo.MaxIterations,
		age,
		loopInfo.UserID)

	return agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		InReplyTo:   targetLoopID,
		Type:        agentic.ResponseTypeStatus,
		Content:     content,
		Timestamp:   time.Now(),
	}, nil
}

// handleLoopsCommand handles the /loops command
func (c *Component) handleLoopsCommand(ctx context.Context, msg agentic.UserMessage, _ []string, _ string) (agentic.UserResponse, error) {
	loops := c.loopTracker.GetUserLoops(msg.UserID)

	c.logger.DebugContext(ctx, "Loops command executed",
		slog.String("user_id", msg.UserID),
		slog.Int("loop_count", len(loops)))

	if len(loops) == 0 {
		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			Type:        agentic.ResponseTypeStatus,
			Content:     "No active loops.",
			Timestamp:   time.Now(),
		}, nil
	}

	var lines []string
	lines = append(lines, "LOOP         STATE       ITER  AGE")
	for _, loop := range loops {
		age := time.Since(loop.CreatedAt).Truncate(time.Second)
		iter := fmt.Sprintf("%d/%d", loop.Iterations, loop.MaxIterations)
		lines = append(lines, fmt.Sprintf("%-12s %-11s %-5s %s",
			truncateID(loop.LoopID),
			loop.State,
			iter,
			formatAge(age)))
	}

	return agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		Type:        agentic.ResponseTypeText,
		Content:     strings.Join(lines, "\n"),
		Timestamp:   time.Now(),
	}, nil
}

// handleHelpCommand handles the /help command
func (c *Component) handleHelpCommand(ctx context.Context, msg agentic.UserMessage, _ []string, _ string) (agentic.UserResponse, error) {
	commands := c.registry.All()

	c.logger.DebugContext(ctx, "Help command executed",
		slog.String("user_id", msg.UserID),
		slog.Int("command_count", len(commands)))

	var lines []string
	lines = append(lines, "Available commands:")
	lines = append(lines, "")

	for _, config := range commands {
		if config.Permission == "" || c.hasPermission(msg.UserID, config.Permission) {
			lines = append(lines, "  "+config.Help)
		}
	}

	lines = append(lines, "")
	lines = append(lines, "Type any other text to submit a task.")

	return agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		Type:        agentic.ResponseTypeText,
		Content:     strings.Join(lines, "\n"),
		Timestamp:   time.Now(),
	}, nil
}

// makeSignalCommand creates a command handler that sends a specific signal type.
// This is a factory to avoid duplicating the same logic for /approve, /reject, /pause, /resume.
func (c *Component) makeSignalCommand(signalType string) CommandHandler {
	return func(ctx context.Context, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
		// Resolve target loop
		targetLoopID := loopID
		if len(args) > 0 && args[0] != "" {
			targetLoopID = args[0]
		}

		if targetLoopID == "" {
			return agentic.UserResponse{
				ResponseID:  uuid.New().String(),
				ChannelType: msg.ChannelType,
				ChannelID:   msg.ChannelID,
				UserID:      msg.UserID,
				Type:        agentic.ResponseTypeError,
				Content:     fmt.Sprintf("No loop to %s. Specify a loop_id or have an active loop.", signalType),
				Timestamp:   time.Now(),
			}, nil
		}

		// Check permission to control this loop
		if !c.canUserControlLoop(msg.UserID, targetLoopID) {
			return agentic.UserResponse{
				ResponseID:  uuid.New().String(),
				ChannelType: msg.ChannelType,
				ChannelID:   msg.ChannelID,
				UserID:      msg.UserID,
				Type:        agentic.ResponseTypeError,
				Content:     "Permission denied: cannot control this loop",
				Timestamp:   time.Now(),
			}, nil
		}

		// Verify loop exists
		loopInfo := c.loopTracker.Get(targetLoopID)
		if loopInfo == nil {
			return agentic.UserResponse{
				ResponseID:  uuid.New().String(),
				ChannelType: msg.ChannelType,
				ChannelID:   msg.ChannelID,
				UserID:      msg.UserID,
				Type:        agentic.ResponseTypeError,
				Content:     fmt.Sprintf("Loop %s not found", targetLoopID),
				Timestamp:   time.Now(),
			}, nil
		}

		if isTerminalState(loopInfo.State) {
			return agentic.UserResponse{
				ResponseID:  uuid.New().String(),
				ChannelType: msg.ChannelType,
				ChannelID:   msg.ChannelID,
				UserID:      msg.UserID,
				Type:        agentic.ResponseTypeStatus,
				Content:     fmt.Sprintf("Loop %s already in terminal state: %s", targetLoopID, loopInfo.State),
				Timestamp:   time.Now(),
			}, nil
		}

		// Build signal with optional reason (args[1] for /approve and /reject)
		signal := agentic.UserSignal{
			SignalID:    uuid.New().String(),
			Type:        signalType,
			LoopID:      targetLoopID,
			UserID:      msg.UserID,
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			Timestamp:   time.Now(),
		}

		// Attach reason as payload if provided (second capture group)
		if len(args) > 1 && args[1] != "" {
			signal.Payload = map[string]string{"reason": strings.TrimSpace(args[1])}
		}

		signalData, err := json.Marshal(signal)
		if err != nil {
			return agentic.UserResponse{}, errs.Wrap(err, "Component", "signalCommand", "marshal signal")
		}

		subject := fmt.Sprintf("agent.signal.%s", targetLoopID)
		if err := c.natsClient.Publish(ctx, subject, signalData); err != nil {
			return agentic.UserResponse{}, errs.WrapTransient(err, "Component", "signalCommand", "publish signal")
		}

		return agentic.UserResponse{
			ResponseID:  uuid.New().String(),
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
			InReplyTo:   targetLoopID,
			Type:        agentic.ResponseTypeStatus,
			Content:     fmt.Sprintf("Signal '%s' sent to loop %s", signalType, targetLoopID),
			Timestamp:   time.Now(),
		}, nil
	}
}

// Helper functions

func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
