package harness

import (
	"strings"
	"testing"
)

func TestRenderResearcherFragment_Empty(t *testing.T) {
	out := RenderResearcherFragment(nil)

	for _, want := range []string{
		"# Available test harnesses",
		"No harnesses are currently registered for this deployment",
		"`configs/harnesses.json`) is empty",
		"needs_harness:",
		"DO NOT add a `needs_harness:` gap",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered fragment missing %q\n--- got ---\n%s", want, out)
		}
	}

	// Should not name a specific harness when empty.
	for _, forbidden := range []string{"meshtasticd", "Image:", "Smoke contract schema"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("rendered empty fragment unexpectedly contains %q", forbidden)
		}
	}
}

func TestRenderResearcherFragment_WithEntries(t *testing.T) {
	catalog := []*Harness{
		{
			Name:                "meshtasticd-3.x",
			ComposeProfile:      "harness-meshtasticd",
			Image:               "meshtastic/meshtasticd:3.5.0",
			SmokeContractSchema: "meshtastic.smoke_contract.v1",
			DomainDescription:   "Real Meshtastic protocol over TCP for protobuf POSITION_APP packets.",
			Exposes: Exposes{
				TCP: []PortExpose{{Port: 4403, Protocol: "meshtastic-protobuf"}},
			},
			RealDependencies: []Dependency{
				{GroupID: "com.geeksville.mesh", ArtifactID: "meshtastic-protobufs", VersionRange: "[2.x,3.x)"},
			},
		},
		{
			Name:                "kafka-stub",
			ComposeProfile:      "harness-kafka",
			Image:               "confluentinc/cp-kafka:7.5.0",
			SmokeContractSchema: "kafka.smoke_contract.v1",
			DomainDescription:   "Single-broker Kafka for topic-shaped consumers.",
		},
	}
	out := RenderResearcherFragment(catalog)

	for _, want := range []string{
		"2 harness(es) registered",
		"1. `meshtasticd-3.x`",
		"2. `kafka-stub`",
		"meshtastic/meshtasticd:3.5.0",
		"meshtastic.smoke_contract.v1",
		"port 4403 (meshtastic-protobuf)",
		"com.geeksville.mesh:meshtastic-protobufs [2.x,3.x)",
		"confluentinc/cp-kafka:7.5.0",
		"kafka.smoke_contract.v1",
		"If NONE of the registered harnesses fits",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered fragment missing %q\n--- got ---\n%s", want, out)
		}
	}

	// kafka-stub has no exposes / no real_dependencies — those sections
	// should be omitted, not rendered as empty bullets.
	kafkaSection := out[strings.Index(out, "kafka-stub"):]
	if strings.Contains(kafkaSection, "TCP surfaces:") {
		t.Errorf("kafka-stub section unexpectedly rendered TCP surfaces (no exposes set)")
	}
	if strings.Contains(kafkaSection, "Real dependencies:") {
		t.Errorf("kafka-stub section unexpectedly rendered Real dependencies (none set)")
	}
}

func TestRenderResearcherFragment_DeterministicAcrossCalls(t *testing.T) {
	catalog := []*Harness{
		{Name: "a", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s.v1", DomainDescription: "d"},
	}
	got1 := RenderResearcherFragment(catalog)
	got2 := RenderResearcherFragment(catalog)
	if got1 != got2 {
		t.Errorf("renderer not deterministic: same input produced different output")
	}
}
