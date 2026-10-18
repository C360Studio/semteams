package main

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/semsource"
	"github.com/c360studio/semteams/internal/runtimecatalog"
)

func TestSetupRegistriesAndManagerRegistersProductCatalog(t *testing.T) {
	cfg := &config.Config{}
	componentRegistry, manager, err := setupRegistriesAndManager(cfg)
	if err != nil {
		t.Fatalf("setupRegistriesAndManager: %v", err)
	}
	if componentRegistry == nil {
		t.Fatal("componentRegistry is nil")
	}
	if manager == nil {
		t.Fatal("service manager is nil")
	}

	got := componentRegistry.ListFactories()
	if len(got) != 27 {
		t.Fatalf("registered factory count = %d, want 27", len(got))
	}

	wantRegistry := component.NewRegistry()
	if err := runtimecatalog.RegisterComponents(wantRegistry); err != nil {
		t.Fatalf("RegisterComponents: %v", err)
	}
	for name := range wantRegistry.ListFactories() {
		if _, ok := got[name]; !ok {
			t.Errorf("setupRegistriesAndManager missing factory %q", name)
		}
	}

	if !cfg.Services["service-manager"].Enabled {
		t.Error("default service-manager config not enabled")
	}
	if !cfg.Services["metrics"].Enabled {
		t.Error("default metrics config not enabled")
	}
}

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

// Exercise the actual bootstrap overlay, including decoding compatibility for
// parked product families; registration alone does not activate their packs.
func TestProductionPayloadRegistryDecodesProductPayloads(t *testing.T) {
	reg, err := buildPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	// StatusPayload is a retained legacy registration but does not implement
	// message.Payload. Keep its factory available without claiming decode proof.
	if payload := reg.Create("semsource", "status", "v1"); reflect.TypeOf(payload) != reflect.TypeOf(&semsource.StatusPayload{}) {
		t.Fatalf("status factory produced %T, want *semsource.StatusPayload", payload)
	}
	payloads := []message.Payload{
		&research.Artifact{LoopID: "research-1", Revision: 1, TestHarness: "catalog-test", ProducedAt: at},
		&devviaspec.Plan{LoopID: "research-1", Revision: 1, Goal: "catalog parity", Context: "frozen baseline", Epics: []string{"registration"}, ProducedAt: at},
		&semsource.EntityPayload{ID: "c360.semteams.source.repo.file.catalog", UpdatedAt: at},
		&semsource.ManifestPayload{Namespace: "semteams", Sources: []semsource.ManifestSource{{Type: "git", Path: "/workspace", Branch: "main"}}, Timestamp: at},
		&semsource.PredicateSchemaPayload{Sources: []semsource.SourcePredicateSchema{{SourceType: "git", Predicates: []semsource.PredicateDescriptor{{Name: "source.file.path", DataType: "string", Role: "location"}}}}, Timestamp: at},
	}
	for _, payload := range payloads {
		t.Run(payload.Schema().Key(), func(t *testing.T) {
			if err := payload.Validate(); err != nil {
				t.Fatalf("invalid fixture: %v", err)
			}
			wire, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "contract-test"))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := message.NewDecoder(reg).Decode(wire)
			if err != nil {
				t.Fatal(err)
			}
			if err := decoded.Validate(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(payload, decoded.Payload()) {
				t.Errorf("decoded %#v, want %#v", decoded.Payload(), payload)
			}
		})
	}
}
