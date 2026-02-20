package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
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
			Error:    "call: NATS client not available",
			Duration: time.Since(start),
		}
	}

	// Parse payload into map for GenericJSONPayload
	dataMap := parsePayloadToMap(a.Payload)

	// Wrap in GenericJSONPayload + BaseMessage envelope
	genericPayload := message.NewGenericJSON(dataMap)
	baseMsg := message.NewBaseMessage(genericPayload.Schema(), genericPayload, "workflow")
	requestPayload, err := json.Marshal(baseMsg)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("call: marshal failed: %v", err),
			Duration: time.Since(start),
		}
	}

	// Perform request with timeout
	response, err := actx.NATSClient.Request(ctx, a.Subject, requestPayload, a.Timeout)
	if err != nil {
		return Result{
			Success:  false,
			Error:    fmt.Sprintf("call: request failed: %v", err),
			Duration: time.Since(start),
		}
	}

	return Result{
		Success:  true,
		Output:   response,
		Duration: time.Since(start),
	}
}
