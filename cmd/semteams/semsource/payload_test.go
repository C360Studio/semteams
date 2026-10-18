package semsource

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

func TestStatusPayloadBaseMessageRoundTrip(t *testing.T) {
	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}

	raw := []byte(`{"id":"00000000-0000-0000-0000-000000000001","type":{"domain":"semsource","category":"status","version":"v1"},"payload":{"status":"ready","sources":2,"entities":42,"uptime":"5m0s"},"meta":{"created_at":1700000000000,"received_at":1700000000000,"source":"semsource-test"}}`)
	decoded, err := message.NewDecoder(reg).Decode(raw)
	if err != nil {
		t.Fatalf("decode semsource.status.v1 BaseMessage: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded Validate: %v", err)
	}
	got, ok := decoded.Payload().(*StatusPayload)
	if !ok {
		t.Fatalf("decoded payload type = %T, want *StatusPayload", decoded.Payload())
	}
	want := &StatusPayload{
		Status:   "ready",
		Sources:  2,
		Entities: 42,
		Uptime:   "5m0s",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded payload = %#v, want %#v", got, want)
	}

	encoded, err := json.Marshal(message.NewBaseMessage(want.Schema(), want, "semsource-test"))
	if err != nil {
		t.Fatalf("marshal BaseMessage: %v", err)
	}
	redecoded, err := message.NewDecoder(reg).Decode(encoded)
	if err != nil {
		t.Fatalf("redecode BaseMessage: %v", err)
	}
	if err := redecoded.Validate(); err != nil {
		t.Fatalf("redecoded Validate: %v", err)
	}
	if !reflect.DeepEqual(redecoded.Payload(), want) {
		t.Fatalf("redecoded payload = %#v, want %#v", redecoded.Payload(), want)
	}
}

func TestStatusPayloadInvalidInputs(t *testing.T) {
	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}
	decoder := message.NewDecoder(reg)

	t.Run("malformed payload json fails unmarshal", func(t *testing.T) {
		raw := []byte(`{"id":"00000000-0000-0000-0000-000000000001","type":{"domain":"semsource","category":"status","version":"v1"},"payload":{"status":123},"meta":{"created_at":1700000000000,"received_at":1700000000000,"source":"semsource-test"}}`)
		_, err := decoder.Decode(raw)
		if err == nil {
			t.Fatal("expected error decoding non-string status field, got nil")
		}
		if strings.Contains(err.Error(), "does not implement message.Payload") {
			t.Fatalf("decode failed on interface check rather than JSON unmarshaling: %v", err)
		}
	})

	cases := []struct {
		name    string
		payload StatusPayload
		wantErr string
	}{
		{
			name:    "empty status rejected",
			payload: StatusPayload{Status: "", Sources: 1, Entities: 10},
			wantErr: "status is required",
		},
		{
			name:    "whitespace-only status rejected",
			payload: StatusPayload{Status: "   ", Sources: 1, Entities: 10},
			wantErr: "status is required",
		},
		{
			name:    "negative sources rejected",
			payload: StatusPayload{Status: "ready", Sources: -1, Entities: 10},
			wantErr: "sources must be non-negative",
		},
		{
			name:    "negative entities rejected",
			payload: StatusPayload{Status: "ready", Sources: 1, Entities: -5},
			wantErr: "entities must be non-negative",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.payload.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate(%#v) error = %v, want substring %q", tc.payload, err, tc.wantErr)
			}

			rawPayload, marshalErr := json.Marshal(&tc.payload)
			if marshalErr != nil {
				t.Fatalf("Marshal(%#v): %v", tc.payload, marshalErr)
			}
			envelope := []byte(`{"id":"00000000-0000-0000-0000-000000000001","type":{"domain":"semsource","category":"status","version":"v1"},"payload":` + string(rawPayload) + `,"meta":{"created_at":1700000000000,"received_at":1700000000000,"source":"semsource-test"}}`)
			decoded, decodeErr := decoder.Decode(envelope)
			if decodeErr != nil {
				t.Fatalf("Decode(%s): %v", envelope, decodeErr)
			}
			if valErr := decoded.Validate(); valErr == nil || !strings.Contains(valErr.Error(), tc.wantErr) {
				t.Fatalf("decoded.Validate() error = %v, want substring %q", valErr, tc.wantErr)
			}
		})
	}
}
