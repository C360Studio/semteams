package evidence

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/c360studio/semteams/cmd/semteams/verification"
)

// Status enumerates the outcomes of a single Checker invocation.
// Closed enum — a fourth state is an architectural decision.
type Status string

// Status values.
const (
	// StatusPass — the checker found the rule satisfied. The
	// rendered evidence summary records this as a satisfied claim.
	StatusPass Status = "pass"

	// StatusFail — the checker ran but the rule was not satisfied
	// (path missing, build tag absent, surefire count short, etc.).
	// The evidence summary records the Detail string for the
	// reviewer's grade.
	StatusFail Status = "fail"

	// StatusUnknownKind — the rule's Kind has no registered
	// Checker. Treated as a rejection by the reviewer (per the
	// validate-vs-gate split documented on EvidenceRule). Distinct
	// from StatusFail so operators reading the summary know the
	// failure mode is "checker missing" not "claim false."
	StatusUnknownKind Status = "unknown_kind"

	// StatusError — the checker errored before reaching a verdict
	// (e.g. corrupted surefire XML, unreadable file under the
	// workspace root). Treated as a rejection; distinct from Fail
	// so a transient I/O failure isn't reported as a falsified
	// claim.
	StatusError Status = "error"
)

// Result is the per-rule outcome the gate emits and the reviewer
// reads. One Result per EvidenceRule on the commitment.
type Result struct {
	// Kind is the rule's Kind string, echoed for the reviewer.
	Kind string

	// Status is the closed-enum verdict.
	Status Status

	// Detail carries the human-readable explanation: "expected ≥3
	// passing surefire tests in com.example.foo, found 1." Empty
	// on Pass; populated on Fail / UnknownKind / Error.
	Detail string
}

// Checker validates a single evidence rule against a Context.
// Implementations are stateless; concurrency-safety is the registry
// + dispatcher's responsibility. A Checker MUST NOT mutate the
// context or the workspace — the gate is read-only.
//
// args is the rule's Args map (raw JSON shape from
// verification.EvidenceRule.Args). Validation of args shape lives
// inside the Checker, not the registry — each kind's args schema
// is kind-specific.
type Checker interface {
	// Check returns the rule's outcome. Errors that prevent reaching
	// a verdict (I/O, parse) should produce StatusError with Detail;
	// genuine rule violations are StatusFail. Returning
	// (Status="", nil) is a programming error — the dispatcher will
	// promote it to StatusError with a sentinel Detail.
	Check(ctx context.Context, args map[string]any, ec *Context) Result
}

// Registry maps Kind strings to Checkers. Concurrency-safe — the
// product-shell may register kinds at boot from multiple init
// functions, and the dispatcher reads concurrently across multiple
// gate runs (one per builder loop reporting in).
//
// The zero Registry is unusable; construct via NewRegistry.
type Registry struct {
	mu       sync.RWMutex
	checkers map[string]Checker
}

// NewRegistry returns an empty Registry ready for Register calls.
func NewRegistry() *Registry {
	return &Registry{checkers: make(map[string]Checker)}
}

// Register associates a Checker with a Kind. Returns an error iff
// the Kind is already registered (caller bug — re-registration
// indicates two init funcs racing for the same Kind, which would
// make outcomes order-dependent).
//
// Kind must match the structural pattern enforced by
// verification.EvidenceRule.Validate (lowercase ASCII + digits +
// underscores; first char a letter). Register echoes a short
// validation here so a typo'd Register call fails at boot rather
// than at the first gate run.
func (r *Registry) Register(kind string, c Checker) error {
	if c == nil {
		return fmt.Errorf("register %q: checker cannot be nil", kind)
	}
	if !isValidKindShape(kind) {
		return fmt.Errorf("register %q: kind must match ^[a-z][a-z0-9_]*$ (matches verification.EvidenceRule.Validate)", kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.checkers[kind]; dup {
		return fmt.Errorf("register %q: kind already registered", kind)
	}
	r.checkers[kind] = c
	return nil
}

// Get returns the Checker for kind, or nil if not registered. The
// nil branch is the gate's "unknown kind" path (StatusUnknownKind);
// callers do NOT distinguish "missing checker" from "checker
// reports unknown" — both are the same diagnostic to the reviewer.
func (r *Registry) Get(kind string) Checker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.checkers[kind]
}

// Kinds returns the registered kinds in sorted order. Used by
// operator-facing introspection (a `/evidence/kinds` HTTP endpoint
// would lift this verbatim) and by the architect tool's static
// instructions fragment when documenting which kinds an operator's
// deployment supports. Sorted for deterministic logging / docs.
func (r *Registry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.checkers))
	for k := range r.checkers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Run dispatches every rule in rules through the registry against
// ec, returning one Result per rule in input order. Returns the
// per-rule Results plus an aggregate count of pass/fail/unknown/
// error so the reviewer can grade at a glance.
//
// The dispatcher does NOT short-circuit: every rule runs even after
// the first failure. The reviewer needs the full picture (all
// failures, not just the first) to grade the commitment honestly
// and to give the builder a complete retry hint.
func (r *Registry) Run(ctx context.Context, rules []verification.EvidenceRule, ec *Context) ([]Result, Aggregate) {
	results := make([]Result, len(rules))
	var agg Aggregate
	for i, rule := range rules {
		results[i] = r.runOne(ctx, rule, ec)
		switch results[i].Status {
		case StatusPass:
			agg.Pass++
		case StatusFail:
			agg.Fail++
		case StatusUnknownKind:
			agg.UnknownKind++
		case StatusError:
			agg.Error++
		default:
			// Defensive: a Checker returning a Status outside the
			// closed enum (typo, future kind that didn't update
			// the dispatcher) would otherwise silently leave the
			// aggregate counters short of agg.Total. Promote to
			// Error so the math stays consistent and the operator
			// sees a loud failure.
			results[i].Status = StatusError
			results[i].Detail = fmt.Sprintf("checker for kind %q returned unrecognised Status %q", rule.Kind, results[i].Status)
			agg.Error++
		}
	}
	agg.Total = len(rules)
	return results, agg
}

// runOne is Run's per-rule helper: looks up the Checker, invokes it,
// promotes a (Status="", nil) return to a sentinel Error so the
// reviewer never sees a half-result.
func (r *Registry) runOne(ctx context.Context, rule verification.EvidenceRule, ec *Context) Result {
	c := r.Get(rule.Kind)
	if c == nil {
		return Result{
			Kind:   rule.Kind,
			Status: StatusUnknownKind,
			Detail: fmt.Sprintf("no Checker registered for kind %q (registered: %v)", rule.Kind, r.Kinds()),
		}
	}
	res := c.Check(ctx, rule.Args, ec)
	if res.Status == "" {
		return Result{
			Kind:   rule.Kind,
			Status: StatusError,
			Detail: fmt.Sprintf("checker for kind %q returned empty Status (programming error)", rule.Kind),
		}
	}
	// Echo the kind even if the checker forgot to set it — operators
	// reading the result want the kind beside every status.
	if res.Kind == "" {
		res.Kind = rule.Kind
	}
	return res
}

// Aggregate is the gate's summary across one commitment's rules.
// Total = Pass + Fail + UnknownKind + Error. Operators logging the
// gate result and the reviewer's grading prompt both consume this
// shape; keeping it as a small struct (not a map) means the field
// names render cleanly in slog output and JSON wire shape.
type Aggregate struct {
	Total       int
	Pass        int
	Fail        int
	UnknownKind int
	Error       int
}

// IsEmpty reports whether the gate ran zero rules. Distinguishing
// empty from "all-passed" matters at the reviewer's call site:
// `if agg.IsEmpty() { flag-as-under-specified } else if !agg.AllPassed()
// { reject }` is the standard shape. The R3.7.2.f′ contract permits
// commitments with empty Evidence at the wire level; the reviewer
// flags them — IsEmpty is the explicit signal.
func (a Aggregate) IsEmpty() bool {
	return a.Total == 0
}

// AllPassed is shorthand for "every rule passed" — the most common
// reviewer query. Returns false when no rules ran (Total=0), so the
// reviewer cannot accidentally accept an empty-Evidence commitment
// via this predicate; pair with IsEmpty when distinguishing the
// two cases matters.
func (a Aggregate) AllPassed() bool {
	return a.Total > 0 && a.Pass == a.Total
}

// isValidKindShape is the small mirror of
// verification.evidenceKindPattern. Kept local so registry is a
// leaf package on the verification primitive (no cycle); the
// pattern is the same one EvidenceRule.Validate enforces.
func isValidKindShape(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
			// always allowed
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		case c == '_':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
