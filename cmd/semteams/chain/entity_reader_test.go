package chain

import "testing"

// TestDecodeExactEntityTriples_Beta160Envelope pins the wire shape of the
// graph.query.entity response at semstreams v1.0.0-beta.160: the ExactEntity
// envelope {entity:{id,triples}, kvRevision}. The fixture mirrors upstream's
// own (processor/graph-query/attack_test.go at the tag). Every consumer unit
// test mocks at the EntityReader interface, so THIS test is the only decode
// coverage against real wire bytes — the pre-160 bare shape decoded to zero
// triples silently and starved autoresearch's empirical compare.
func TestDecodeExactEntityTriples_Beta160Envelope(t *testing.T) {
	fixture := []byte(`{"entity":{"id":"c360.demo.agent.chain.execution.loop_1","triples":[` +
		`{"subject":"c360.demo.agent.chain.execution.loop_1","predicate":"autoresearch.best.value","object":1.2},` +
		`{"subject":"c360.demo.agent.chain.execution.loop_1","predicate":"autoresearch.run.status","object":"active"}` +
		`]},"kvRevision":7}`)

	triples, err := DecodeExactEntityTriples(fixture)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if got := triples["autoresearch.best.value"]; got != 1.2 {
		t.Errorf("best.value = %v, want 1.2", got)
	}
	if got := triples["autoresearch.run.status"]; got != "active" {
		t.Errorf("run.status = %v, want active", got)
	}
}

// TestDecodeExactEntityTriples_Bare159ShapeYieldsEmpty documents the drift
// class: the pre-beta.160 bare {id,triples} shape is NOT an error — it
// decodes to an empty map. If this test starts failing because the bare
// shape suddenly parses, the responder contract changed again; re-verify
// against the pinned tag before adjusting either way.
func TestDecodeExactEntityTriples_Bare159ShapeYieldsEmpty(t *testing.T) {
	bare := []byte(`{"id":"c360.demo.agent.chain.execution.loop_1","triples":[` +
		`{"predicate":"autoresearch.best.value","object":1.2}]}`)

	triples, err := DecodeExactEntityTriples(bare)
	if err != nil {
		t.Fatalf("bare shape should not error: %v", err)
	}
	if len(triples) != 0 {
		t.Errorf("bare 159 shape decoded %d triples; the envelope contract must have changed — re-verify upstream", len(triples))
	}
}
