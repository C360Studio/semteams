package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Canonical predicate contract audit (upstream ADR-074, enforced fail-closed
// at persistence since semstreams beta.147-150).
//
// Every stored predicate must be exactly three dot-segments, each lower-kebab
// ([a-z][a-z0-9]*(-[a-z0-9]+)*, ≤64 bytes/segment, ≤194 bytes total). There
// is NO alias mode and NO escape hatch: graph-ingest rejects a non-conforming
// write, graph-index goes sticky-degraded on reading one, and GET
// /graph/triples hard-fails the whole request on a poisoned entity. Rule-pack
// JSON never recompiles, so predicate strings in configs are the silent
// casualty class — this audit makes the flag-day a CI failure instead.
//
// Scope: the BOOTSTRAP-WIRED rule files + both flow configs' projection
// contracts. Parked packs (ADR-058) stay on disk in the pre-migration dialect
// and are deliberately NOT audited — re-wiring one without re-authoring it
// makes this test fail, which is exactly the fence.

var canonicalSegment = `[a-z][a-z0-9]*(-[a-z0-9]+)*`
var canonicalPredicateRe = regexp.MustCompile(`^` + canonicalSegment + `(\.` + canonicalSegment + `){2}$`)

// substitution accessors that may legally trail a predicate inside a
// $entity.triple.<predicate>[.<accessor>] token.
var tripleAccessors = map[string]bool{"length": true, "triples": true, "value": true}

var entityTripleTokenRe = regexp.MustCompile(`\$entity\.triple\.([a-zA-Z0-9_.-]+)`)

func validateCanonicalPredicate(t *testing.T, where, predicate string) {
	t.Helper()
	if !canonicalPredicateRe.MatchString(predicate) {
		t.Errorf("%s: predicate %q violates the canonical predicate contract "+
			"(exactly 3 dot-segments, lower-kebab, no underscores)", where, predicate)
		return
	}
	if len(predicate) > 194 {
		t.Errorf("%s: predicate %q exceeds 194 bytes", where, predicate)
	}
	for _, seg := range strings.Split(predicate, ".") {
		if len(seg) > 64 {
			t.Errorf("%s: predicate %q segment %q exceeds 64 bytes", where, predicate, seg)
		}
	}
}

// validateTripleToken validates the predicate embedded in a
// $entity.triple.<predicate>[.<accessor>] substitution token.
func validateTripleToken(t *testing.T, where, token string) {
	t.Helper()
	segs := strings.Split(token, ".")
	// Trailing punctuation from prose (e.g. "…$entity.triple.agent.run.phase.")
	// never appears in functional fields; functional tokens are exact.
	switch {
	case len(segs) == 3:
		validateCanonicalPredicate(t, where, token)
	case len(segs) == 4 && tripleAccessors[segs[3]]:
		validateCanonicalPredicate(t, where, strings.Join(segs[:3], "."))
	default:
		t.Errorf("%s: $entity.triple token %q is neither a 3-segment predicate "+
			"nor a predicate + accessor (.length/.triples/.value)", where, token)
	}
}

// auditRule recursively walks a rule document's functional fields.
func auditRule(t *testing.T, path string, node any, inConditions bool) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			switch key {
			case "field":
				if s, ok := val.(string); ok && !strings.HasPrefix(s, "$") {
					validateCanonicalPredicate(t, path+" condition field", s)
				}
			case "predicate":
				if s, ok := val.(string); ok {
					validateCanonicalPredicate(t, path+" action predicate", s)
				}
			case "related_loops":
				if m, ok := val.(map[string]any); ok {
					for k := range m {
						// Keys become the third segment of
						// agent.lineage.<key> — one kebab segment.
						if !regexp.MustCompile(`^` + canonicalSegment + `$`).MatchString(k) {
							t.Errorf("%s: related_loops key %q must be a single lower-kebab segment "+
								"(it becomes agent.lineage.%s)", path, k, k)
						}
					}
				}
			case "description", "metadata", "name":
				continue // prose — not functional
			}
			auditRule(t, path, val, inConditions || key == "conditions")
		}
	case []any:
		for _, item := range v {
			auditRule(t, path, item, inConditions)
		}
	case string:
		// Functional strings may embed $entity.triple.<predicate> tokens
		// (subjects, objects, prompts). Prose keys were skipped above.
		for _, m := range entityTripleTokenRe.FindAllStringSubmatch(v, -1) {
			validateTripleToken(t, path, strings.TrimSuffix(m[1], "."))
		}
	}
}

// stripProse removes the non-functional prose fields from a decoded rule so
// the string-token walk only sees functional surfaces.
func stripProse(node any) any {
	m, ok := node.(map[string]any)
	if !ok {
		return node
	}
	out := map[string]any{}
	for k, v := range m {
		if k == "description" || k == "metadata" || k == "_comment" {
			continue
		}
		if mm, ok := v.(map[string]any); ok {
			out[k] = stripProse(mm)
			continue
		}
		if arr, ok := v.([]any); ok {
			outArr := make([]any, 0, len(arr))
			for _, item := range arr {
				outArr = append(outArr, stripProse(item))
			}
			out[k] = outArr
			continue
		}
		out[k] = v
	}
	return out
}

func TestWiredRulePredicatesAreCanonical(t *testing.T) {
	for _, path := range bootstrapLoadedRules(t) {
		raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		auditRule(t, path, stripProse(doc), false)
	}
}

func TestProjectionContractPredicatesAreCanonical(t *testing.T) {
	for _, flowPath := range []string{flowBootstrapPath, e2eFlowBootstrapPath} {
		cfg := loadRuleProcessorOwnership(t, flowPath)
		for _, c := range cfg.ProjectionContracts {
			for gi, g := range c.Groups {
				for _, p := range g.Predicates {
					validateCanonicalPredicate(t,
						fmt.Sprintf("%s contract %q group %d", flowPath, c.Name, gi), p)
				}
			}
		}
	}
}
