package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestConfigGraphPairing asserts the structural invariant that any
// agentic config including `graph-ingest` (the write-side processor
// that handles graph.mutation.triple.add and serves entity reads on
// graph.ingest.query.entity) ALSO includes `graph-query` (the
// passthrough read-API processor that owns the graph.query.* subject
// namespace our chain milestone subscribers and chainpause decision
// handler request against).
//
// Why this exists: smoke #8 run-4 wedged the chain entity propagation
// because every agentic config had graph-ingest but no graph-query —
// our cmd/semteams/chain/resolver.go and chainpause/decision_handler.go
// requested entity reads on graph.query.entity (the upstream graph-
// query processor's subject) and timed out silently with no responder.
// The bug is structural: graph-ingest without graph-query means the
// product shell cannot READ what it WRITES.
//
// The inverse (graph-query without graph-ingest) is also broken since
// graph-query is a passthrough router that forwards entity queries to
// graph.ingest.query.entity (see processor/graph-query/router.go
// StaticRouter "entity" -> "graph.ingest.query.entity"). graph-query
// alone has nothing behind it.
//
// Configs that have NEITHER are fine — they're stateless / thin demo
// configs that don't exercise graph features. The pairing invariant
// only fires when one is present.
//
// Five configs deliberately have neither (onboarding.json,
// e2e-agentic.json, e2e-deep-research.json, e2e-ops-observer.json,
// e2e-research-with-source.json) — those are intentional minimalism
// that the post-smoke-#8 audit deferred per-config decision on. If
// any of them grows graph-ingest later, this test catches the missing
// graph-query at commit time.
func TestConfigGraphPairing(t *testing.T) {
	t.Parallel()

	matches, err := filepath.Glob("../../configs/*.json")
	if err != nil {
		t.Fatalf("glob configs/*.json: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no configs found under configs/*.json")
	}

	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var cfg struct {
				Components map[string]any `json:"components"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}

			_, hasIngest := cfg.Components["graph-ingest"]
			_, hasQuery := cfg.Components["graph-query"]

			switch {
			case hasIngest && !hasQuery:
				t.Errorf("%s has graph-ingest but not graph-query — chain milestone subscribers (cmd/semteams/chain/) and chainpause decision handler (cmd/semteams/chainpause/) request entity reads on the graph.query.* namespace; without graph-query they time out silently with no responder. Add graph-query alongside graph-ingest. (See smoke #8 run-4 root cause.)", filepath.Base(path))
			case hasQuery && !hasIngest:
				t.Errorf("%s has graph-query but not graph-ingest — graph-query is a passthrough router that forwards entity queries to graph.ingest.query.entity (see upstream processor/graph-query/router.go StaticRouter). Without graph-ingest the requests have nowhere to land. Add graph-ingest alongside graph-query.", filepath.Base(path))
			}
			// hasIngest && hasQuery: paired correctly, OK.
			// !hasIngest && !hasQuery: stateless config, OK.
		})
	}
}
