package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
)

func TestProductionPayloadRegistryDecodesTypedUserResponse(t *testing.T) {
	reg, err := buildPayloadRegistry()
	if err != nil {
		t.Fatalf("build payload registry: %v", err)
	}

	response := &agentic.UserResponse{
		ResponseID:  "response-1",
		ChannelType: "web",
		ChannelID:   "channel-1",
		Type:        agentic.ResponseTypeText,
		Content:     "program pulse ready",
		Timestamp:   time.Now().UTC(),
	}
	encoded, err := json.Marshal(message.NewBaseMessage(response.Schema(), response, "contract-test"))
	if err != nil {
		t.Fatalf("marshal typed user response: %v", err)
	}

	decoded, err := message.NewDecoder(reg).Decode(encoded)
	if err != nil {
		t.Fatalf("decode typed user response with production registry: %v", err)
	}
	if _, ok := decoded.Payload().(*agentic.UserResponse); !ok {
		t.Fatalf("decoded payload type = %T, want *agentic.UserResponse", decoded.Payload())
	}
}
