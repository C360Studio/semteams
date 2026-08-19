package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic/agentrun"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
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

// TestWiredReplaceOwnedGroupsAreSinglePredicate pins the beta.159 semantics
// flip that wedged autoresearch (go-reviewer C1): ReplaceOwned reconciles the
// WHOLE selected group (RemoveTriples = every group predicate), so a group
// shared by independent single-predicate stamps is a mutual-destruction lane.
// Every wired replace_owned action's selected group must contain exactly the
// action's own predicate; a deliberate multi-predicate reconcile group would
// need one action authoring ALL of its triples, which the rule action shape
// cannot express today.
func TestWiredReplaceOwnedGroupsAreSinglePredicate(t *testing.T) {
	cfg := loadRuleProcessorOwnership(t, flowBootstrapPath)
	groups := map[string][]string{}
	for _, c := range cfg.ProjectionContracts {
		for _, g := range c.Groups {
			groups[c.Name+"/"+g.Name] = g.Predicates
		}
	}
	for _, path := range bootstrapLoadedRules(t) {
		raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule struct {
			OnEnter []struct {
				Type               string `json:"type"`
				Predicate          string `json:"predicate"`
				ProjectionContract string `json:"projection_contract"`
				ProjectionGroup    string `json:"projection_group"`
			} `json:"on_enter"`
		}
		if json.Unmarshal(raw, &rule) != nil {
			continue
		}
		for _, a := range rule.OnEnter {
			if a.Type != "reconcile_predicates" {
				continue
			}
			key := a.ProjectionContract + "/" + a.ProjectionGroup
			preds, ok := groups[key]
			if !ok {
				t.Errorf("%s: replace_owned selects undeclared group %q", path, key)
				continue
			}
			if len(preds) != 1 || preds[0] != a.Predicate {
				t.Errorf("%s: replace_owned on %q selects group %q holding %v — beta.159 reconciles the whole group, so this action would erase its siblings",
					path, a.Predicate, key, preds)
			}
		}
	}
}

// TestBootstrapRuleListsMatch pins that the mock-LLM clone loads exactly the
// production rule set — a pack wired in one but not the other would pass mock
// journeys yet diverge in production (or vice versa).
func TestBootstrapRuleListsMatch(t *testing.T) {
	load := func(path string) map[string]bool {
		raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var cfg struct {
			Components map[string]struct {
				Config struct {
					RulesFiles []string `json:"rules_files"`
				} `json:"config"`
			} `json:"components"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		out := map[string]bool{}
		for _, f := range cfg.Components["rule"].Config.RulesFiles {
			out[f] = true
		}
		return out
	}
	prod, e2e := load(flowBootstrapPath), load(e2eFlowBootstrapPath)
	for f := range prod {
		if !e2e[f] {
			t.Errorf("rule %s wired in flow-bootstrap but missing from e2e-flow-bootstrap", f)
		}
	}
	for f := range e2e {
		if !prod[f] {
			t.Errorf("rule %s wired in e2e-flow-bootstrap but missing from flow-bootstrap", f)
		}
	}
}

// keptEmitterFiles are the kept-lane Go writers whose predicate constants the
// canonical contract must hold for (go-reviewer C2 was emit_plan escaping the
// sweep). Parked-lane emitters are intentionally absent — see ADR-058.
var keptEmitterFiles = []string{
	"../../cmd/semteams/tools/emitartifact/executor.go",
	"../../cmd/semteams/tools/emitautoresearchbaseline/executor.go",
	"../../cmd/semteams/tools/emitautoresearchmeasurement/executor.go",
	"../../cmd/semteams/tools/emitautoresearchartifact/executor.go",
	"../../cmd/semteams/tools/emitplan/executor.go",
	"../../cmd/semteams/sandboxmanager/attestation.go",
	"../../cmd/semteams/approvalpause/pauser.go",
	"../../cmd/semteams/chainpause/pauser.go",
	"../../cmd/semteams/chainpause/decision_handler.go",
}

// emitterNonPredicates are predicate-shaped strings in the audited files that
// are deliberately NOT graph predicates (NATS subjects, payload type keys).
var emitterNonPredicates = map[string]bool{
	"dev_via_spec.plan.v1": true, // payload type key (wire namespace, not a predicate)
	"research.artifact.v1": true, // payload type key
	"graph.query.entity":   true, // NATS request subject
}

var goPredicateShapedRe = regexp.MustCompile(`"([a-z][a-z0-9_-]*\.[a-z0-9_-]+\.[a-z0-9_.-]+?)"`)

// TestKeptEmitterPredicatesAreCanonical audits the kept-lane Go writers'
// string literals: anything shaped like a stored predicate must satisfy the
// canonical grammar. Runtime persistence is fail-closed on shape (not
// registration), so a stale constant here means silently rejected writes.
func TestKeptEmitterPredicatesAreCanonical(t *testing.T) {
	for _, path := range keptEmitterFiles {
		raw, err := os.ReadFile(path) //nolint:gosec // test-controlled path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range goPredicateShapedRe.FindAllStringSubmatch(string(raw), -1) {
			lit := m[1]
			if emitterNonPredicates[lit] || strings.Contains(lit, "..") {
				continue
			}
			if strings.HasSuffix(lit, "-") || strings.HasSuffix(lit, ".") {
				// dynamic prefix constant (e.g. sandbox.attestation.verified-):
				// validate with a probe suffix
				validateCanonicalPredicate(t, path+" (prefix constant)", lit+"probe")
				continue
			}
			validateCanonicalPredicate(t, path, lit)
		}
	}
}

// wiredPersonaDirs are the persona fragment directories loaded for live roles.
var wiredPersonaDirs = []string{
	"../../configs/personas/fragments/coordinator",
	"../../configs/personas/fragments/researcher-research-plan",
	"../../configs/personas/fragments/researcher-research-gather",
	"../../configs/personas/fragments/researcher-research-synthesize",
	"../../configs/personas/fragments/reviewer-research",
	"../../configs/personas/fragments/reviewer-autoresearch",
	"../../configs/personas/fragments/autoresearch-baseline",
	"../../configs/personas/fragments/autoresearch-propose",
	"../../configs/personas/fragments/autoresearch-execute",
	"../../configs/personas/fragments/autoresearch-synthesize",
	"../../configs/personas/fragments/ops-chain-observer",
	"../../configs/personas/fragments-autonomous/coordinator",
}

// TestLoopFamilyRulesGateOnRole fences the structural property that keeps the
// ops observer from observing itself, replacing the denylist the ops pack used
// to carry (which had already rotted — it named two roles that no longer
// existed).
//
// The ops observer fires on the RUN family and spawns a loop-family entity. It
// is safe from self-observation only because every wired loop-family rule
// gates on an explicit agent.loop.role naming a non-ops role. A future rule
// keyed on, say, agent.loop.outcome alone would silently capture ops loops —
// and since the ops loop's own decide stamps a loop-family entity, that is a
// spawn cycle: observer terminal fires observer.
//
// Nothing else catches this. TestWiredRulesNarrowEntityPattern covers the
// diagnosis-entity half (an ops.diagnosis.* prefix is unclassifiable and so
// must be recorded explicitly), but not this one.
func TestLoopFamilyRulesGateOnRole(t *testing.T) {
	for _, path := range bootstrapLoadedRules(t) {
		raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule struct {
			Type   string `json:"type"`
			Entity struct {
				Pattern string `json:"pattern"`
			} `json:"entity"`
			Conditions []struct {
				Field string `json:"field"`
				Value any    `json:"value"`
			} `json:"conditions"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if rule.Type == "cron" || rule.Entity.Pattern != loopEntityPattern {
			continue
		}

		var gated bool
		for _, c := range rule.Conditions {
			if c.Field != "agent.loop.role" {
				continue
			}
			gated = true
			for _, v := range roleValues(c.Value) {
				if strings.HasPrefix(v, "ops-") {
					t.Errorf("%s: loop-family rule names ops role %q — an ops loop's own terminal "+
						"would re-trigger observation, spawning loops without bound", path, v)
				}
			}
		}
		if !gated {
			t.Errorf("%s: loop-family rule has no agent.loop.role condition, so it evaluates against "+
				"EVERY loop including ops observers. Add an explicit role gate", path)
		}
	}
}

// TestWiredPersonaTripleTokensAreCanonical audits $entity.triple.* tokens in
// live persona prose — the LLM echoes these into tool calls and reads, so a
// stale token is a silent behavioral bug the rule audit cannot see.
func TestWiredPersonaTripleTokensAreCanonical(t *testing.T) {
	for _, dir := range wiredPersonaDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := dir + "/" + e.Name()
			raw, err := os.ReadFile(path) //nolint:gosec // test-controlled path
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			for _, m := range entityTripleTokenRe.FindAllStringSubmatch(string(raw), -1) {
				token := strings.TrimSuffix(strings.TrimSuffix(m[1], "."), "*")
				token = strings.TrimSuffix(token, ".")
				// placeholder forms (<cap>, {a,b}) are prose, not literals
				if strings.ContainsAny(token, "{<") || token == "" {
					continue
				}
				// dynamic-prefix prose (…verified-<probe>): the capture stops
				// at '<', leaving a trailing hyphen — validate with a probe
				if strings.HasSuffix(token, "-") {
					validateTripleToken(t, path+" (dynamic prefix)", token+"probe")
					continue
				}
				validateTripleToken(t, path, token)
			}
		}
	}
}

// TestWiredRulesDeclareEntityPattern pins the beta.159 silent-skip class the
// research journey caught live: evaluateRulesForEntityState SKIPS any rule
// whose definition lacks entity.pattern (beta.115 evaluated pattern-less
// rules against every entity update). A wired rule without a pattern loads
// green, seeds green, and simply never fires — the user waits forever.
func TestWiredRulesDeclareEntityPattern(t *testing.T) {
	for _, path := range bootstrapLoadedRules(t) {
		raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule struct {
			Type   string `json:"type"`
			Entity struct {
				Pattern string `json:"pattern"`
			} `json:"entity"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if rule.Type == "cron" {
			continue
		}
		if rule.Entity.Pattern == "" {
			t.Errorf("%s: expression rule missing entity.pattern — the beta.159 EntityState evaluator silently skips it", path)
		}
	}
}

// The two entity families every wired rule fires on. Loop entities carry the
// agent.loop.* / coordinator.decision.* facts; run entities carry agent.run.*.
//
// The run pattern comes from upstream so a family rename breaks compilation
// instead of silently diverging. The loop pattern has no exported equivalent
// (its source of truth, semstreams internal/builtinprojection/contracts.go,
// is an internal package), so it stays a literal.
const loopEntityPattern = "*.*.agent.agentic-loop.execution.*"

var runEntityPattern = agentrun.EntityIDPattern

// Wired rules whose conditions read no predicate the family classifier
// recognizes, mapped to the fence that covers their entity.pattern instead.
// Keep this as small as the classifier allows — an entry is an admission that
// this test cannot check the file.
var unclassifiedRuleFamilies = map[string]string{
	// Conditions are autoresearch.run.* / autoresearch.iteration.*. Its
	// reconcile_predicates actions omit `subject`, so the firing family flows
	// into upstream's boot-time containment check — mirrored by
	// TestReconcile_ResolvedTargetsWithinContractScope, which fails loudly if
	// this pattern moves off the run family.
	"05-iteration-dispatch.json": "TestReconcile_ResolvedTargetsWithinContractScope",
}

// TestWiredRulesNarrowEntityPattern pins the successor to the blanket
// `*.*.*.*.*.*` that ADR-058 finding 6 introduced as a stopgap. The blanket
// made every wired rule evaluate its full condition set against every entity
// snapshot the ENTITY_STATES watcher delivers, and it is outright illegal for
// any rule whose reconcile_predicates targets the firing entity — boot-time
// containment (upstream processor/rule/projection_derivation.go
// validateDeclaredProjectionSuperset) demands contract entity_pattern ⊇ rule
// entity.pattern and fail-closes at load. Finding 9 already hit that live.
//
// The pattern is derivable from the rule's own conditions, so this asserts the
// coupling rather than a hand-maintained list: a rule reading agent.loop.* /
// coordinator.decision.* / agent.lineage.* fires on the loop entity; one
// reading only agent.run.* fires on the run entity.
//
// entity_watch_buckets containment is checked here too — a rule pattern the
// component's watch patterns do not cover never receives a snapshot, which is
// the same silent-never-fires failure as a missing pattern.
func TestWiredRulesNarrowEntityPattern(t *testing.T) {
	watchPatterns := ruleWatchPatterns(t)
	for _, path := range bootstrapLoadedRules(t) {
		raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule struct {
			Type   string `json:"type"`
			Entity struct {
				Pattern string `json:"pattern"`
			} `json:"entity"`
			Conditions []struct {
				Field string `json:"field"`
			} `json:"conditions"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if rule.Type == "cron" {
			continue
		}
		pattern := rule.Entity.Pattern
		if pattern == "*.*.*.*.*.*" {
			t.Errorf("%s: entity.pattern is the blanket *.*.*.*.*.* — narrow it to the entity family "+
				"the rule's conditions read (%s or %s). The blanket evaluates the rule against every "+
				"entity update and fail-closes rule load when a reconcile_predicates action targets the "+
				"firing entity", path, loopEntityPattern, runEntityPattern)
			continue
		}

		var readsLoop, readsRun bool
		for _, c := range rule.Conditions {
			switch {
			// agent.run.entity-id is a LOOP-entity predicate despite the
			// agent.run.* prefix: it is the pointer triple the rule engine
			// stamps on the FIRING loop (upstream actions.go stampRun), and
			// upstream declares it among the loop predicates. Classifying it
			// by prefix would force a rule that reads only this field onto
			// the run pattern, where it could never fire. Sibling
			// agent.run.* fields really are run-side, so do not widen this.
			case c.Field == agvocab.LoopRunEntityID:
				readsLoop = true
			case strings.HasPrefix(c.Field, "agent.loop."),
				strings.HasPrefix(c.Field, "coordinator.decision."),
				strings.HasPrefix(c.Field, "agent.lineage."):
				readsLoop = true
			case strings.HasPrefix(c.Field, "agent.run."):
				readsRun = true
			}
		}
		switch {
		case readsLoop && pattern != loopEntityPattern:
			t.Errorf("%s: conditions read loop-entity facts but entity.pattern is %q, want %q",
				path, pattern, loopEntityPattern)
		case !readsLoop && readsRun && pattern != runEntityPattern:
			t.Errorf("%s: conditions read only run-entity facts but entity.pattern is %q, want %q",
				path, pattern, runEntityPattern)
		case !readsLoop && !readsRun:
			// Abstaining here is how a future category pack (ops.diagnosis.*,
			// a new pack's namespace) would sail through unchecked. Make the
			// unrecognized prefix a decision point instead. autoresearch/05
			// is the live example: its firing family is pinned by
			// TestReconcile_ResolvedTargetsWithinContractScope, which mirrors
			// the boot-time containment check, so name it here explicitly.
			if _, known := unclassifiedRuleFamilies[filepath.Base(path)]; !known {
				t.Errorf("%s: conditions read no known entity-family predicate, so this test cannot "+
					"verify entity.pattern %q names the family the rule actually fires on. Add the "+
					"predicate prefix to the classifier, or record the file in "+
					"unclassifiedRuleFamilies with the fence that covers it", path, pattern)
			}
		}

		// Reachability must hold under EVERY bootstrap, not merely one of
		// them — the journeys run the e2e config, production runs the other.
		for configPath, patterns := range watchPatterns {
			if !anyWatchPatternReaches(patterns, pattern) {
				t.Errorf("%s: entity.pattern %q cannot be reached by any %s entity_watch_buckets "+
					"pattern %v — the watcher never delivers a matching snapshot and the rule never fires",
					path, pattern, configPath, patterns)
			}
		}
	}
}

// ruleWatchPatterns returns the rule processor's configured ENTITY_STATES
// watch patterns. Both bootstraps are read and their patterns unioned: the
// journeys run against e2e-flow-bootstrap.json, and TestBootstrapRuleListsMatch
// pins rules_files across the two but not entity_watch_buckets, so a rule
// reachable in production could be unreachable under the e2e config.
func ruleWatchPatterns(t *testing.T) map[string][]string {
	t.Helper()
	all := map[string][]string{}
	for _, path := range []string{
		"../../configs/flow-bootstrap.json",
		"../../configs/e2e-flow-bootstrap.json",
	} {
		data, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var bootstrap struct {
			Components map[string]struct {
				Config struct {
					EntityWatchBuckets map[string][]string `json:"entity_watch_buckets"`
				} `json:"config"`
			} `json:"components"`
		}
		if err := json.Unmarshal(data, &bootstrap); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		patterns := bootstrap.Components["rule"].Config.EntityWatchBuckets["ENTITY_STATES"]
		if len(patterns) == 0 {
			t.Fatalf("%s: rule-processor entity_watch_buckets.ENTITY_STATES is empty — no rule can ever fire", path)
		}
		all[path] = patterns
	}
	return all
}

// anyWatchPatternReaches reports whether some watch pattern can deliver an
// entity the rule pattern matches. Both are six-position dotted globs, and the
// relation that matters is INTERSECTION, not containment: rule patterns leave
// the org/platform positions as `*` (upstream's own convention — see
// internal/builtinprojection contracts) while the watch pattern pins the
// deployment org, so a containment test would demand a literal org in all 41
// rule files for no benefit. What this catches is the reachability failure —
// a rule naming an entity family whose literal segments the watcher excludes.
func anyWatchPatternReaches(watchPatterns []string, rulePattern string) bool {
	ruleParts := strings.Split(rulePattern, ".")
	for _, watch := range watchPatterns {
		watchParts := strings.Split(watch, ".")
		if len(watchParts) != len(ruleParts) {
			// Different segment counts can never describe the same ID, so
			// this watch pattern cannot reach the rule. Skipping is the
			// answer, not an omission: if every watch pattern mismatches,
			// the loop ends and reports unreachable.
			continue
		}
		reachable := true
		for i := range watchParts {
			if watchParts[i] != "*" && ruleParts[i] != "*" && watchParts[i] != ruleParts[i] {
				reachable = false
				break
			}
		}
		if reachable {
			return true
		}
	}
	return false
}
