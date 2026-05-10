package testharness

import (
	"strings"
	"testing"
)

func TestRenderResearcherFragment_Empty(t *testing.T) {
	out := RenderResearcherFragment(nil)

	for _, want := range []string{
		"# Available test harnesses",
		"No test harnesses are currently registered for this deployment",
		"`configs/harnesses.json`) is empty",
		"needs_test_harness:",
		// Smoke #8 run-9: empty-catalog branch now points pure work
		// at the "not applicable" escape hatch (was: "DO NOT add a
		// needs_test_harness: gap" — the now-rejected shape).
		"needs_test_harness: not applicable",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered fragment missing %q\n--- got ---\n%s", want, out)
		}
	}

	// Should not name a specific test harness when empty.
	for _, forbidden := range []string{"meshtasticd", "Image:", "Smoke contract schema"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("rendered empty fragment unexpectedly contains %q", forbidden)
		}
	}
}

func TestRenderResearcherFragment_WithEntries(t *testing.T) {
	catalog := []*TestHarness{
		{
			Name:                "meshtasticd-2.x",
			ComposeProfile:      "harness-meshtasticd",
			Image:               "meshtastic/meshtasticd:2.7.23-alpine",
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
		"2 test harness(es) registered",
		"1. `meshtasticd-2.x`",
		"2. `kafka-stub`",
		"meshtastic/meshtasticd:2.7.23-alpine",
		"meshtastic.smoke_contract.v1",
		"port 4403 (meshtastic-protobuf)",
		"com.geeksville.mesh:meshtastic-protobufs [2.x,3.x)",
		"confluentinc/cp-kafka:7.5.0",
		"kafka.smoke_contract.v1",
		"If NONE of the registered test harnesses fits",
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
	catalog := []*TestHarness{
		{Name: "a", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s.v1", DomainDescription: "d"},
	}
	got1 := RenderResearcherFragment(catalog)
	got2 := RenderResearcherFragment(catalog)
	if got1 != got2 {
		t.Errorf("renderer not deterministic: same input produced different output")
	}
}
