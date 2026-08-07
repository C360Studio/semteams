// Package teamhint wires the public team slash commands (/research,
// /optimize) as coordinator-routed hints.
//
// beta.159's agentic-dispatch treats EVERY leading-slash message as a command
// lookup and rejects unknown names ("Unknown command") — the beta.115
// behavior where an unregistered "/research …" fell through to the
// coordinator as plain chat is gone. The product contract (docs: "slash
// commands are coordinator-routed hints, not bypasses") therefore needs the
// hint commands REGISTERED, each re-entering the normal front door: the
// executor rewrites "/team rest…" to "@team rest…" (the @-prefix hint form
// the coordinator persona already teaches) and republishes the message onto
// the USER stream, where dispatch consumes it as an ordinary task
// submission. No routing decision happens here — the coordinator still owns
// classification, and a hint never bypasses its validation.
package teamhint

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
)

// publishSource tags the republished envelope for log/provenance greps.
const publishSource = "semteams-team-hint"

// Command is one registered team-hint slash command.
type Command struct {
	name   string // slash-command name, e.g. "research"
	team   string // canonical hint token the content is rewritten to
	help   string
	logger *slog.Logger
}

// New builds a team-hint command. name is the slash token (without the
// slash); team is the canonical @-hint the content is rewritten to (usually
// the same string; aliases like /autoresearch → optimize differ).
func New(name, team, help string, logger *slog.Logger) *Command {
	if logger == nil {
		logger = slog.Default()
	}
	return &Command{name: name, team: team, help: help, logger: logger}
}

// Config implements agenticdispatch.CommandExecutor.
func (c *Command) Config() agenticdispatch.CommandConfig {
	return agenticdispatch.CommandConfig{
		// (?s) so a multi-line prompt stays one hint body.
		Pattern:     `(?s)^/` + c.name + `\s+(.+)$`,
		Permission:  "submit_task",
		RequireLoop: false,
		Help:        c.help,
	}
}

// Execute implements agenticdispatch.CommandExecutor: rewrite the hint to
// the @-prefix form and republish through the USER stream front door.
func (c *Command) Execute(
	ctx context.Context,
	cmdCtx *agenticdispatch.CommandContext,
	msg agentic.UserMessage,
	args []string,
	_ string,
) (agentic.UserResponse, error) {
	body := ""
	if len(args) > 0 {
		body = strings.TrimSpace(args[0])
	}
	if body == "" {
		return c.response(msg, agentic.ResponseTypeError,
			fmt.Sprintf("Usage: /%s <request>", c.name)), nil
	}
	if cmdCtx == nil || cmdCtx.NATSClient == nil {
		return c.response(msg, agentic.ResponseTypeError,
			"Team routing is unavailable right now (no message bus)."), nil
	}

	hinted := msg
	hinted.MessageID = uuid.NewString()
	hinted.Content = "@" + c.team + " " + body

	envelope := message.NewBaseMessage(hinted.Schema(), &hinted, publishSource)
	data, err := json.Marshal(envelope)
	if err != nil {
		return agentic.UserResponse{}, fmt.Errorf("marshal hinted user message: %w", err)
	}

	subject := "user.message." + msg.ChannelType
	if _, err := cmdCtx.NATSClient.PublishToStreamWithAck(ctx, subject, data); err != nil {
		return agentic.UserResponse{}, fmt.Errorf("republish team hint to %s: %w", subject, err)
	}

	c.logger.Info("team hint routed through front door",
		slog.String("command", c.name),
		slog.String("team", c.team),
		slog.String("message_id", hinted.MessageID))

	return c.response(msg, agentic.ResponseTypeStatus,
		fmt.Sprintf("Routing your /%s request through the coordinator…", c.name)), nil
}

func (c *Command) response(msg agentic.UserMessage, kind, content string) agentic.UserResponse {
	return agentic.UserResponse{
		ResponseID:  uuid.New().String(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		Type:        kind,
		Content:     content,
		Timestamp:   time.Now(),
	}
}
