// Package analyzeproof implements the deterministic proof-readiness analyzer
// for spec-driven development runs. It reads proof.* facts from a run entity
// and projects them into formal_claims.* findings that rules and the UI can
// consume without asking an LLM to judge infrastructure state.
package analyzeproof

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semstreams/message"
)

const (
	analyzerVersion = "go-native-v1"
	toolSource      = "proof-readiness-analyzer"

	statusPassed    = "passed"
	statusFailed    = "failed"
	statusAmbiguous = "ambiguous"

	severityBlocker = "blocker"
	severityWarning = "warning"

	routeCoordinator    = "coordinator"
	routeTestHarness    = "test_harness"
	routeImplementation = "implementation"
	routePause          = "pause"
)

var (
	claimFields = []string{
		"source_requirement", "source_scenario", "conflicts_with", "task_refs",
		"statement", "requires", "status",
	}
	dependencyFields = []string{
		"required_for", "profile_ref", "next_route", "description", "kind", "status",
	}
	readinessFields = []string{
		"failure_signature", "attestation_ref", "probe_results", "smoke_command",
		"smoke_status", "profile_ref", "completed_at", "started_at", "expires_at",
		"evidence", "status",
	}
	evidenceFields = []string{
		"created_at", "exit_code", "producer", "command", "digest", "covers", "kind", "uri",
	}
	waiverFields = []string{
		"residual_risk", "approved_by", "approved_at", "expires_at", "dependencies",
		"claims", "reason", "status",
	}
)

// Claim is the proof.claim.<id>.* input record.
type Claim struct {
	ID                string
	Statement         string
	SourceRequirement string
	SourceScenario    string
	Requires          []string
	ConflictsWith     []string
	Status            string
	TaskRefs          []string
}

// Dependency is the proof.dependency.<id>.* input record.
type Dependency struct {
	ID          string
	Kind        string
	Description string
	RequiredFor []string
	Status      string
	ProfileRef  string
	NextRoute   string
}

// Readiness is the proof.readiness.<id>.* input record.
type Readiness struct {
	ID               string
	ProfileRef       string
	Status           string
	StartedAt        string
	CompletedAt      string
	ExpiresAt        string
	ProbeResults     string
	SmokeCommand     string
	SmokeStatus      string
	AttestationRef   string
	Evidence         []string
	FailureSignature string
}

// Evidence is the proof.evidence.<id>.* input record.
type Evidence struct {
	ID        string
	Kind      string
	URI       string
	Digest    string
	Producer  string
	Command   string
	ExitCode  string
	CreatedAt string
	Covers    []string
}

// Waiver is the proof.waiver.<id>.* input record.
type Waiver struct {
	ID           string
	Reason       string
	ApprovedBy   string
	ApprovedAt   string
	ExpiresAt    string
	Claims       []string
	Dependencies []string
	ResidualRisk string
	Status       string
}

// ProofFacts is the analyzer's parsed input graph.
type ProofFacts struct {
	Claims       map[string]*Claim
	Dependencies map[string]*Dependency
	Readiness    map[string]*Readiness
	Evidence     map[string]*Evidence
	Waivers      map[string]*Waiver
}

// Analysis is the routeable formal_claims.* projection.
type Analysis struct {
	Status   string    `json:"status"`
	Version  string    `json:"version"`
	Findings []Finding `json:"findings"`
}

// Finding is one deterministic blocker/warning emitted by the analyzer.
type Finding struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Route      string `json:"route"`
	Reason     string `json:"reason"`
	Claim      string `json:"claim,omitempty"`
	Dependency string `json:"dependency,omitempty"`
	Profile    string `json:"profile,omitempty"`
	Readiness  string `json:"readiness,omitempty"`
	Waiver     string `json:"waiver,omitempty"`
}

// ParseProofFacts maps run-entity predicates into typed proof records. Unknown
// proof.* predicates are ignored so the model can grow without breaking older
// analyzers.
func ParseProofFacts(triples map[string]any) ProofFacts {
	f := ProofFacts{
		Claims:       map[string]*Claim{},
		Dependencies: map[string]*Dependency{},
		Readiness:    map[string]*Readiness{},
		Evidence:     map[string]*Evidence{},
		Waivers:      map[string]*Waiver{},
	}
	for pred, obj := range triples {
		if id, field, ok := splitProofPredicate(pred, "proof.claim.", claimFields); ok {
			c := ensureClaim(f.Claims, id)
			switch field {
			case "statement":
				c.Statement = stringValue(obj)
			case "source_requirement":
				c.SourceRequirement = stringValue(obj)
			case "source_scenario":
				c.SourceScenario = stringValue(obj)
			case "requires":
				c.Requires = stringList(obj)
			case "conflicts_with":
				c.ConflictsWith = stringList(obj)
			case "status":
				c.Status = normalizeStatus(stringValue(obj))
			case "task_refs":
				c.TaskRefs = stringList(obj)
			}
			continue
		}
		if id, field, ok := splitProofPredicate(pred, "proof.dependency.", dependencyFields); ok {
			d := ensureDependency(f.Dependencies, id)
			switch field {
			case "kind":
				d.Kind = stringValue(obj)
			case "description":
				d.Description = stringValue(obj)
			case "required_for":
				d.RequiredFor = stringList(obj)
			case "status":
				d.Status = normalizeStatus(stringValue(obj))
			case "profile_ref":
				d.ProfileRef = stringValue(obj)
			case "next_route":
				d.NextRoute = normalizeRoute(stringValue(obj))
			}
			continue
		}
		if id, field, ok := splitProofPredicate(pred, "proof.readiness.", readinessFields); ok {
			r := ensureReadiness(f.Readiness, id)
			switch field {
			case "profile_ref":
				r.ProfileRef = stringValue(obj)
			case "status":
				r.Status = normalizeStatus(stringValue(obj))
			case "started_at":
				r.StartedAt = stringValue(obj)
			case "completed_at":
				r.CompletedAt = stringValue(obj)
			case "expires_at":
				r.ExpiresAt = stringValue(obj)
			case "probe_results":
				r.ProbeResults = stringValue(obj)
			case "smoke_command":
				r.SmokeCommand = stringValue(obj)
			case "smoke_status":
				r.SmokeStatus = normalizeStatus(stringValue(obj))
			case "attestation_ref":
				r.AttestationRef = stringValue(obj)
			case "evidence":
				r.Evidence = stringList(obj)
			case "failure_signature":
				r.FailureSignature = stringValue(obj)
			}
			continue
		}
		if id, field, ok := splitProofPredicate(pred, "proof.evidence.", evidenceFields); ok {
			ev := ensureEvidence(f.Evidence, id)
			switch field {
			case "kind":
				ev.Kind = stringValue(obj)
			case "uri":
				ev.URI = stringValue(obj)
			case "digest":
				ev.Digest = stringValue(obj)
			case "producer":
				ev.Producer = stringValue(obj)
			case "command":
				ev.Command = stringValue(obj)
			case "exit_code":
				ev.ExitCode = stringValue(obj)
			case "created_at":
				ev.CreatedAt = stringValue(obj)
			case "covers":
				ev.Covers = stringList(obj)
			}
			continue
		}
		if id, field, ok := splitProofPredicate(pred, "proof.waiver.", waiverFields); ok {
			w := ensureWaiver(f.Waivers, id)
			switch field {
			case "reason":
				w.Reason = stringValue(obj)
			case "approved_by":
				w.ApprovedBy = stringValue(obj)
			case "approved_at":
				w.ApprovedAt = stringValue(obj)
			case "expires_at":
				w.ExpiresAt = stringValue(obj)
			case "claims":
				w.Claims = stringList(obj)
			case "dependencies":
				w.Dependencies = stringList(obj)
			case "residual_risk":
				w.ResidualRisk = stringValue(obj)
			case "status":
				w.Status = normalizeStatus(stringValue(obj))
			}
		}
	}
	return f
}

// Analyze evaluates proof facts deterministically. It does not create proof;
// it only reports whether existing graph facts are enough to route onward.
func Analyze(triples map[string]any, now time.Time) Analysis {
	facts := ParseProofFacts(triples)
	return AnalyzeFacts(facts, now)
}

// AnalyzeFacts evaluates an already-parsed proof fact model. It is split from
// Analyze so callers that also need fact summaries do not parse the graph
// twice.
func AnalyzeFacts(facts ProofFacts, now time.Time) Analysis {
	a := Analysis{Status: statusPassed, Version: analyzerVersion}

	accepted := acceptedClaimIDs(facts.Claims)
	if len(accepted) == 0 {
		a.addFinding(Finding{
			Kind:     "no_accepted_claims",
			Severity: severityBlocker,
			Route:    routeCoordinator,
			Reason:   "no proof.claim records with status accepted were found on the run entity",
		})
		a.Status = statusAmbiguous
		return a
	}

	for _, claimID := range accepted {
		claim := facts.Claims[claimID]
		a.evaluateConflicts(facts, claim)
		if len(claim.Requires) == 0 && !evidenceCoversClaim(facts, claimID) {
			if active, inactive := matchingWaiver(facts, claimID, "", now); active != nil {
				continue
			} else if inactive != nil {
				a.addInactiveWaiverFinding(claimID, "", inactive)
			} else {
				a.addFinding(Finding{
					Kind:     "unproved_claim",
					Severity: severityBlocker,
					Route:    routeTestHarness,
					Claim:    claimID,
					Reason:   "accepted claim has no proof dependencies, covering evidence, or active waiver",
				})
			}
			continue
		}
		for _, depID := range claim.Requires {
			a.evaluateDependency(facts, claimID, depID, now)
		}
	}

	if hasBlocker(a.Findings) {
		a.Status = statusFailed
	} else {
		a.Status = statusPassed
	}
	return a
}

// Triples renders the analysis as formal_claims.* facts on one run entity.
func (a Analysis) Triples(runEntityID string, now time.Time) []message.Triple {
	out := make([]message.Triple, 0, 4+len(a.Findings)*10)
	mk := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject: runEntityID, Predicate: pred, Object: obj, Source: toolSource,
			Timestamp: now, Confidence: 1.0,
		}
	}
	out = append(out,
		mk("formal_claims.status", a.Status),
		mk("formal_claims.analyzer.version", a.Version),
		mk("formal_claims.analyzed_at", now.Format(time.RFC3339Nano)),
		mk("formal_claims.finding_count", strconv.Itoa(len(a.Findings))),
	)
	for _, route := range a.routes() {
		out = append(out, mk("formal_claims.route."+route, "present"))
	}
	for _, f := range a.Findings {
		prefix := "formal_claims.finding." + f.ID + "."
		out = append(out,
			mk(prefix+"kind", f.Kind),
			mk(prefix+"severity", f.Severity),
			mk(prefix+"route", f.Route),
			mk(prefix+"reason", f.Reason),
		)
		if f.Claim != "" {
			out = append(out, mk(prefix+"claim", f.Claim))
		}
		if f.Dependency != "" {
			out = append(out, mk(prefix+"dependency", f.Dependency))
		}
		if f.Profile != "" {
			out = append(out, mk(prefix+"profile", f.Profile))
		}
		if f.Readiness != "" {
			out = append(out, mk(prefix+"readiness", f.Readiness))
		}
		if f.Waiver != "" {
			out = append(out, mk(prefix+"waiver", f.Waiver))
		}
	}
	return out
}

func (a Analysis) routes() []string {
	set := map[string]bool{}
	if a.Status == statusPassed {
		set[routeImplementation] = true
	}
	for _, f := range a.Findings {
		if f.Route != "" {
			set[f.Route] = true
		}
	}
	if a.Status == statusFailed && len(set) == 0 {
		set[routePause] = true
	}
	routes := make([]string, 0, len(set))
	for route := range set {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	return routes
}

func (a *Analysis) evaluateConflicts(facts ProofFacts, claim *Claim) {
	for _, otherID := range claim.ConflictsWith {
		if other, ok := facts.Claims[otherID]; ok && other.Status == "accepted" {
			a.addFinding(Finding{
				Kind:     "conflicting_accepted_claim",
				Severity: severityBlocker,
				Route:    routeCoordinator,
				Claim:    claim.ID,
				Reason:   fmt.Sprintf("accepted claim conflicts with accepted claim %q", otherID),
			})
		}
	}
}

func (a *Analysis) evaluateDependency(facts ProofFacts, claimID, depID string, now time.Time) {
	dep, ok := facts.Dependencies[depID]
	if !ok {
		if active, inactive := matchingWaiver(facts, claimID, depID, now); active != nil {
			return
		} else if inactive != nil {
			a.addInactiveWaiverFinding(claimID, depID, inactive)
			return
		}
		a.addFinding(Finding{
			Kind:       "missing_proof_dependency",
			Severity:   severityBlocker,
			Route:      routeTestHarness,
			Claim:      claimID,
			Dependency: depID,
			Reason:     "accepted claim requires a dependency that has no proof.dependency record",
		})
		return
	}

	route := dep.NextRoute
	if route == "" {
		route = routeTestHarness
	}
	if active, inactive := matchingWaiver(facts, claimID, depID, now); active != nil {
		return
	} else if dep.Status == "waived" || inactive != nil {
		if inactive != nil {
			a.addInactiveWaiverFinding(claimID, depID, inactive)
		} else {
			a.addFinding(Finding{
				Kind:       "missing_waiver",
				Severity:   severityBlocker,
				Route:      routeCoordinator,
				Claim:      claimID,
				Dependency: depID,
				Reason:     "dependency is marked waived but no active proof.waiver covers it",
			})
		}
		return
	}

	switch dep.Status {
	case "ready":
		if dep.ProfileRef == "" {
			return
		}
		if finding, ok := readinessFinding(facts, claimID, dep, now); ok {
			a.addFinding(finding)
		}
	case "missing", "unknown", "":
		a.addFinding(Finding{
			Kind:       "missing_proof_dependency",
			Severity:   severityBlocker,
			Route:      route,
			Claim:      claimID,
			Dependency: depID,
			Reason:     "required proof dependency is not ready",
		})
	case "failed":
		a.addFinding(Finding{
			Kind:       "failed_proof_dependency",
			Severity:   severityBlocker,
			Route:      route,
			Claim:      claimID,
			Dependency: depID,
			Reason:     "required proof dependency is marked failed",
		})
	default:
		a.addFinding(Finding{
			Kind:       "unknown_proof_dependency_status",
			Severity:   severityBlocker,
			Route:      routeCoordinator,
			Claim:      claimID,
			Dependency: depID,
			Reason:     fmt.Sprintf("dependency has unsupported status %q", dep.Status),
		})
	}
}

func (a *Analysis) addInactiveWaiverFinding(claimID, depID string, waiver *Waiver) {
	kind := "inactive_waiver"
	if waiver.Status == "expired" {
		kind = "expired_waiver"
	} else if waiver.Status == "revoked" {
		kind = "revoked_waiver"
	} else if waiver.Status == "invalid_expiry" {
		kind = "invalid_waiver_expiry"
	}
	a.addFinding(Finding{
		Kind:       kind,
		Severity:   severityBlocker,
		Route:      routeCoordinator,
		Claim:      claimID,
		Dependency: depID,
		Waiver:     waiver.ID,
		Reason:     "matching waiver exists but is not active for this run",
	})
}

func (a *Analysis) addFinding(f Finding) {
	f.ID = fmt.Sprintf("f%03d", len(a.Findings)+1)
	if f.Route == "" {
		f.Route = routeCoordinator
	}
	if f.Severity == "" {
		f.Severity = severityWarning
	}
	a.Findings = append(a.Findings, f)
}

func readinessFinding(facts ProofFacts, claimID string, dep *Dependency, now time.Time) (Finding, bool) {
	records := readinessForProfile(facts.Readiness, dep.ProfileRef)
	if len(records) == 0 {
		return Finding{
			Kind:       "missing_readiness_record",
			Severity:   severityBlocker,
			Route:      routeTestHarness,
			Claim:      claimID,
			Dependency: dep.ID,
			Profile:    dep.ProfileRef,
			Reason:     "dependency references a harness profile with no readiness record",
		}, true
	}
	var stale *Readiness
	var failed *Readiness
	var blocked *Readiness
	var invalidExpiry *Readiness
	for _, r := range records {
		expired, invalid := expiredAt(r.ExpiresAt, now)
		switch {
		case invalid:
			invalidExpiry = r
		case r.Status == "passed" && r.SmokeStatus != "failed" && !expired:
			return Finding{}, false
		case r.Status == "failed" || r.SmokeStatus == "failed":
			failed = r
		case expired || r.Status == "stale":
			stale = r
		case r.Status == "blocked":
			blocked = r
		}
	}
	if failed != nil {
		return Finding{
			Kind:       "failed_readiness_record",
			Severity:   severityBlocker,
			Route:      routeTestHarness,
			Claim:      claimID,
			Dependency: dep.ID,
			Profile:    dep.ProfileRef,
			Readiness:  failed.ID,
			Reason:     "matching readiness evidence failed",
		}, true
	}
	if invalidExpiry != nil {
		return Finding{
			Kind:       "invalid_readiness_expiry",
			Severity:   severityBlocker,
			Route:      routeTestHarness,
			Claim:      claimID,
			Dependency: dep.ID,
			Profile:    dep.ProfileRef,
			Readiness:  invalidExpiry.ID,
			Reason:     "readiness record has an invalid expires_at timestamp",
		}, true
	}
	if stale != nil {
		return Finding{
			Kind:       "stale_readiness_record",
			Severity:   severityBlocker,
			Route:      routeTestHarness,
			Claim:      claimID,
			Dependency: dep.ID,
			Profile:    dep.ProfileRef,
			Readiness:  stale.ID,
			Reason:     "readiness record is stale or expired",
		}, true
	}
	if blocked != nil {
		return Finding{
			Kind:       "blocked_readiness_record",
			Severity:   severityBlocker,
			Route:      routeTestHarness,
			Claim:      claimID,
			Dependency: dep.ID,
			Profile:    dep.ProfileRef,
			Readiness:  blocked.ID,
			Reason:     "readiness record is blocked",
		}, true
	}
	return Finding{
		Kind:       "unknown_readiness_status",
		Severity:   severityBlocker,
		Route:      routeTestHarness,
		Claim:      claimID,
		Dependency: dep.ID,
		Profile:    dep.ProfileRef,
		Reason:     "no matching readiness record is passed and fresh",
	}, true
}

func readinessForProfile(readiness map[string]*Readiness, profile string) []*Readiness {
	var records []*Readiness
	for _, r := range readiness {
		if r.ProfileRef == profile {
			records = append(records, r)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records
}

func matchingWaiver(facts ProofFacts, claimID, depID string, now time.Time) (active, inactive *Waiver) {
	ids := mapKeys(facts.Waivers)
	for _, id := range ids {
		w := facts.Waivers[id]
		if !waiverCovers(w, claimID, depID) {
			continue
		}
		expired, invalid := expiredAt(w.ExpiresAt, now)
		if w.Status == "active" && !expired && !invalid {
			return w, nil
		}
		if inactive == nil {
			copy := *w
			if invalid {
				copy.Status = "invalid_expiry"
			} else if expired {
				copy.Status = "expired"
			}
			inactive = &copy
		}
	}
	return nil, inactive
}

func waiverCovers(w *Waiver, claimID, depID string) bool {
	if claimID != "" && contains(w.Claims, claimID) {
		return true
	}
	if depID != "" && contains(w.Dependencies, depID) {
		return true
	}
	return false
}

func expiredAt(raw string, now time.Time) (expired, invalid bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, false
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return false, true
	}
	return now.After(t), false
}

func evidenceCoversClaim(facts ProofFacts, claimID string) bool {
	for _, ev := range facts.Evidence {
		if contains(ev.Covers, claimID) {
			return true
		}
	}
	return false
}

func acceptedClaimIDs(claims map[string]*Claim) []string {
	var ids []string
	for id, c := range claims {
		if c.Status == "accepted" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func hasBlocker(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == severityBlocker {
			return true
		}
	}
	return false
}

func splitProofPredicate(pred, prefix string, fields []string) (id, field string, ok bool) {
	rest, ok := strings.CutPrefix(pred, prefix)
	if !ok {
		return "", "", false
	}
	for _, field := range fields {
		suffix := "." + field
		if strings.HasSuffix(rest, suffix) {
			id := strings.TrimSuffix(rest, suffix)
			if id != "" {
				return id, field, true
			}
		}
	}
	return "", "", false
}

func ensureClaim(m map[string]*Claim, id string) *Claim {
	if c, ok := m[id]; ok {
		return c
	}
	c := &Claim{ID: id}
	m[id] = c
	return c
}

func ensureDependency(m map[string]*Dependency, id string) *Dependency {
	if d, ok := m[id]; ok {
		return d
	}
	d := &Dependency{ID: id}
	m[id] = d
	return d
}

func ensureReadiness(m map[string]*Readiness, id string) *Readiness {
	if r, ok := m[id]; ok {
		return r
	}
	r := &Readiness{ID: id}
	m[id] = r
	return r
}

func ensureEvidence(m map[string]*Evidence, id string) *Evidence {
	if ev, ok := m[id]; ok {
		return ev
	}
	ev := &Evidence{ID: id}
	m[id] = ev
	return ev
}

func ensureWaiver(m map[string]*Waiver, id string) *Waiver {
	if w, ok := m[id]; ok {
		return w
	}
	w := &Waiver{ID: id}
	m[id] = w
	return w
}

func normalizeStatus(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeRoute(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

func stringList(v any) []string {
	switch x := v.(type) {
	case []string:
		return cleanStrings(x)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, stringValue(item))
		}
		return cleanStrings(out)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		if strings.HasPrefix(s, "[") {
			var xs []string
			if err := json.Unmarshal([]byte(s), &xs); err == nil {
				return cleanStrings(xs)
			}
			var anyList []any
			if err := json.Unmarshal([]byte(s), &anyList); err == nil {
				out := make([]string, 0, len(anyList))
				for _, item := range anyList {
					out = append(out, stringValue(item))
				}
				return cleanStrings(out)
			}
		}
		return []string{s}
	case nil:
		return nil
	default:
		return []string{stringValue(x)}
	}
}

func cleanStrings(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func mapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
