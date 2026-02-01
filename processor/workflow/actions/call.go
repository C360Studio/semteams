package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CallAction performs a NATS request/response
type CallAction struct {
	Subject string
	Payload json.RawMessage
	Timeout time.Duration
}

// NewCallAction creates a new call action
func NewCallAction(subject string, payload json.RawMessage, timeout time.Duration) *CallAction {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &CallAction{
		Subject: subject,
		Payload: payload,
		Timeout: timeout,
	}
}

// Execute performs the NATS request and waits for response
func (a *CallAction) Execute(ctx context.Context, actx *Context) Result {
	start := time.Now()

	if actx.NATSClient == nil {
		return Result{
			Success:  false,
			Error:    "NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Prepare payload
	payload := a.Payload
	if payload == nil {
		payload = []byte("{}")
	}

	// Perform request with timeout
	response, err := actx.NATSClient.Request(ctx, a.Subject, payload, a.Timeout)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("request failed: %v", err),
			Duration: time.Since(start),
		}
	}

	return Result{
		Success:  true,
		Output:   response,
		Duration: time.Since(start),
	}
}
