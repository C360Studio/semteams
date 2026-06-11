// Package querysandboxattestation implements the read-only side of
// the ADR-043 attestation contract: given a SandboxRequirements +
// matched profile, look up whether a fresh attestation already
// covers the request and return its shape plus a staleness flag.
//
// Coordinator persona calls this BEFORE request_sandbox to avoid
// re-attesting when a recent attestation on the same chain
// (sandbox.attestation.signature == SignatureFor(profile,
// requirements_hash)) is still fresh. Saves the
// `devcontainer up + probe` cost when the answer hasn't changed.
//
// Read-only: no triple stamps, no container side effects, no
// devcontainer-cli invocation. Pure entity-triple lookup +
// freshness arithmetic.
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): no upstream tool reads
// product-specific triples. Defensible product-shell-local
// (verified semstreams beta.86, 2026-05-29). Migration target:
// graduates alongside request_sandbox if either ever ports
// upstream.
package querysandboxattestation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/runanchor"
	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
)

// ToolName is the LLM-facing tool name.
const ToolName = "query_sandbox_attestation"

// EntityTripleReader matches the shape used by chain.NATSEntityReader.
// Declared here so the package doesn't import that whole subsystem
// just for the reader interface.
type EntityTripleReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// Executor implements agentic.ToolExecutor for
// query_sandbox_attestation.
type Executor struct {
	catalog  *sandboxmanager.Catalog
	entities EntityTripleReader
	platform types.PlatformMeta
	now      func() time.Time
	logger   *slog.Logger
}

// NewExecutor constructs an Executor. catalog + entities + platform are
// boot-required. The chain entity ID is read off the ToolCall's run anchor
// (ADR-053 Phase 5 / semstreams#250) — no Resolver. now defaults to time.Now
// (UTC) when nil.
func NewExecutor(catalog *sandboxmanager.Catalog, entities EntityTripleReader, platform types.PlatformMeta, now func() time.Time, logger *slog.Logger) *Executor {
	if catalog == nil {
		panic("querysandboxattestation.NewExecutor: catalog must not be nil")
	}
	if entities == nil {
		panic("querysandboxattestation.NewExecutor: entities reader must not be nil")
	}
	if platform.Org == "" || platform.Platform == "" {
		panic("querysandboxattestation.NewExecutor: platform.Org and platform.Platform must be set")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{catalog: catalog, entities: entities, platform: platform, now: now, logger: logger}
}

// ListTools returns the LLM-facing schema. Args are a subset of
// SandboxRequirements — only the capability-shape inputs that
// affect the signature. Coordinator calls this with the same
// requirements it would pass to request_sandbox; the tool computes
// the signature and looks it up.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Read-only: check whether a fresh sandbox attestation already covers a SandboxRequirements on this chain. Returns {found, fresh, attestation?, staleness_seconds?}. " +
			"Call BEFORE request_sandbox to skip re-attesting when a recent attestation matches. The signature key is (matched_profile, requirements_hash); pass the same args you would pass to request_sandbox. " +
			"No side effects: no devcontainer up, no triple stamps, just a graph lookup.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"languages":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Same shape as request_sandbox.languages."},
				"tools":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Same shape as request_sandbox.tools."},
				"services":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Same shape as request_sandbox.services."},
				"network":    map[string]any{"type": "string", "enum": []string{"restricted", "public", "none"}, "description": "Same shape as request_sandbox.network."},
				"secrets":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Same shape as request_sandbox.secrets."},
				"mounts":     map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"workspace-write", "workspace-read"}}, "description": "Same shape as request_sandbox.mounts."},
				"privileges": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"docker-socket", "privileged"}}, "description": "Same shape as request_sandbox.privileges."},
				"verification": map[string]any{
					"type":        "array",
					"description": "Same shape as request_sandbox.verification. Affects the requirements hash.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":        map[string]any{"type": "string"},
							"command":     map[string]any{"type": "string"},
							"expect_exit": map[string]any{"type": "integer"},
						},
						"required": []string{"name", "command"},
					},
				},
			},
		},
	}}
}

// Execute parses → matches profile → computes signature → reads
// chain entity → returns lookup result.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name), nil
	}
	req, err := parseRequirements(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err), nil
	}

	// Match the profile BEFORE looking up — the signature is keyed
	// on (profile, requirements_hash). If the catalog has no match,
	// the answer is "not found" with the structured unmet list.
	profile, err := sandboxmanager.MatchProfile(req, e.catalog)
	if err != nil {
		body, _ := json.Marshal(noMatchResponse{Found: false, NoMatch: true, Reason: err.Error()})
		return agentic.ToolResult{
			CallID:  call.ID,
			Name:    call.Name,
			Content: string(body),
			Metadata: map[string]any{
				"found":    false,
				"no_match": true,
			},
		}, nil
	}
	signature := sandboxmanager.SignatureFor(profile, req)

	chainEntityID, err := e.chainEntityFromCall(call)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "%v", err), nil
	}

	triples, err := e.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "read chain entity %s: %v", chainEntityID, err), nil
	}

	att, found := attestationFromTriples(triples, signature)
	fresh := false
	var stalenessSec int64
	if found {
		now := e.now()
		fresh = att.IsFresh(now)
		if !att.AttestedAt.IsZero() {
			stalenessSec = int64(now.Sub(att.AttestedAt).Seconds())
		}
	}

	resp := lookupResponse{
		Found:            found,
		Fresh:            fresh,
		StalenessSeconds: stalenessSec,
		ProfileMatch:     profile.ID(),
		Signature:        signature,
	}
	if found {
		w := attToWire(att)
		resp.Attestation = &w
	}

	body, _ := json.Marshal(resp)
	e.logger.Info("query_sandbox_attestation",
		slog.String("chain_entity", chainEntityID),
		slog.String("profile", profile.ID()),
		slog.String("signature", signature),
		slog.Bool("found", found),
		slog.Bool("fresh", fresh),
		slog.Int64("staleness_s", stalenessSec))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"found":     found,
			"fresh":     fresh,
			"signature": signature,
			"profile":   profile.ID(),
		},
	}, nil
}

// chainEntityFromCall resolves the chain entity ID via runanchor.ChainEntityID:
// related_loops["chain-entity-id"] first, else the dispatch-stamped run anchor on
// ToolCall.Metadata (ADR-053 Phase 5 / semstreams#250 — no Resolver walk).
func (e *Executor) chainEntityFromCall(call agentic.ToolCall) (string, error) {
	entityID, err := runanchor.ChainEntityID(call, e.platform.Org, e.platform.Platform)
	if err != nil {
		return "", fmt.Errorf("query_sandbox_attestation: %w", err)
	}
	return entityID, nil
}

// attestationFromTriples reconstructs an Attestation from chain
// entity triples IF the stamped signature matches the query
// signature. Returns (zero, false) when:
//   - No sandbox.attestation.signature triple is present
//   - The stamped signature differs from the query (a different
//     SandboxRequirements was previously requested on this chain)
//
// Wire-form fields that don't reconstruct cleanly (e.g. Verified
// map split across per-key triples) are best-effort reassembled.
func attestationFromTriples(triples map[string]any, querySignature string) (sandboxmanager.Attestation, bool) {
	if triples == nil {
		return sandboxmanager.Attestation{}, false
	}
	stampedSig, _ := triples[sandboxmanager.PredicateAttestationSignature].(string)
	if stampedSig == "" || stampedSig != querySignature {
		return sandboxmanager.Attestation{}, false
	}
	att := sandboxmanager.Attestation{
		ProfileID:        asString(triples[sandboxmanager.PredicateAttestationProfile]),
		ImageDigest:      asString(triples[sandboxmanager.PredicateAttestationImageDigest]),
		Ready:            asBool(triples[sandboxmanager.PredicateAttestationReady]),
		Degraded:         asBool(triples[sandboxmanager.PredicateAttestationDegraded]),
		Terminal:         asBool(triples[sandboxmanager.PredicateAttestationTerminal]),
		TerminalReason:   asString(triples[sandboxmanager.PredicateAttestationTerminalReason]),
		AdmissionOutcome: sandboxmanager.AdmissionOutcome(asString(triples[sandboxmanager.PredicateAttestationAdmission])),
		RequirementsHash: asString(triples[sandboxmanager.PredicateAttestationRequirementsHash]),
		Signature:        stampedSig,
		Verified:         map[string]string{},
	}
	if s := asString(triples[sandboxmanager.PredicateAttestationAttestedAt]); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			att.AttestedAt = t.UTC()
		} else if t, err := time.Parse(time.RFC3339, s); err == nil {
			att.AttestedAt = t.UTC()
		}
	}
	if ttlSec, ok := triples[sandboxmanager.PredicateAttestationTTL]; ok {
		switch v := ttlSec.(type) {
		case float64:
			att.TTL = time.Duration(v) * time.Second
		case int64:
			att.TTL = time.Duration(v) * time.Second
		case int:
			att.TTL = time.Duration(v) * time.Second
		case json.Number:
			n, _ := v.Int64()
			att.TTL = time.Duration(n) * time.Second
		}
	}
	att.DegradedReasons = asStringSlice(triples[sandboxmanager.PredicateAttestationDegradedReasons])
	att.FailedProbes = asStringSlice(triples[sandboxmanager.PredicateAttestationFailedProbes])
	att.AdmissionReasons = asStringSlice(triples[sandboxmanager.PredicateAttestationAdmissionReasons])
	for k, v := range triples {
		if !strings.HasPrefix(k, sandboxmanager.PredicateAttestationVerifiedPrefix) {
			continue
		}
		name := strings.TrimPrefix(k, sandboxmanager.PredicateAttestationVerifiedPrefix)
		if name == "" {
			continue
		}
		att.Verified[name] = asString(v)
	}
	return att, true
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// asStringSlice decodes a triple Object that should be a []string.
// NATS triple ingest JSON-encodes the Object, so a Go []string
// round-trips back as []any of strings. Handles the rare legacy
// case where the Object came back as a plain string by returning
// a single-element slice (best-effort backward compat).
func asStringSlice(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []string:
		if len(x) == 0 {
			return nil
		}
		return append([]string(nil), x...)
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	default:
		return nil
	}
}

// parseRequirements delegates to sandboxmanager.ParseRequirements —
// the shared parser between this tool and request_sandbox. The two
// tools MUST accept identical inputs so the (profile,
// requirements_hash) signature round-trips without drift.
func parseRequirements(raw map[string]any) (sandboxmanager.SandboxRequirements, error) {
	return sandboxmanager.ParseRequirements(raw)
}

// attestationWire mirrors requestsandbox.attestationWire so the two
// tools return the same shape. Coordinator persona can branch
// identically on either tool's `attestation` body.
type attestationWire struct {
	Profile          string            `json:"profile"`
	ImageDigest      string            `json:"image_digest,omitempty"`
	Ready            bool              `json:"ready"`
	Degraded         bool              `json:"degraded"`
	Terminal         bool              `json:"terminal"`
	TerminalReason   string            `json:"terminal_reason,omitempty"`
	AdmissionOutcome string            `json:"admission_outcome"`
	AdmissionReasons []string          `json:"admission_reasons,omitempty"`
	DegradedReasons  []string          `json:"degraded_reasons,omitempty"`
	FailedProbes     []string          `json:"failed_probes,omitempty"`
	Verified         map[string]string `json:"verified,omitempty"`
	RequirementsHash string            `json:"requirements_hash"`
	Signature        string            `json:"signature"`
	AttestedAt       string            `json:"attested_at"`
	TTLSeconds       int64             `json:"ttl_seconds"`
}

func attToWire(a sandboxmanager.Attestation) attestationWire {
	return attestationWire{
		Profile:          a.ProfileID,
		ImageDigest:      a.ImageDigest,
		Ready:            a.Ready,
		Degraded:         a.Degraded,
		Terminal:         a.Terminal,
		TerminalReason:   a.TerminalReason,
		AdmissionOutcome: string(a.AdmissionOutcome),
		AdmissionReasons: a.AdmissionReasons,
		DegradedReasons:  a.DegradedReasons,
		FailedProbes:     a.FailedProbes,
		Verified:         a.Verified,
		RequirementsHash: a.RequirementsHash,
		Signature:        a.Signature,
		AttestedAt:       a.AttestedAt.Format(time.RFC3339Nano),
		TTLSeconds:       int64(a.TTL.Seconds()),
	}
}

type lookupResponse struct {
	Found            bool             `json:"found"`
	Fresh            bool             `json:"fresh"`
	StalenessSeconds int64            `json:"staleness_seconds,omitempty"`
	ProfileMatch     string           `json:"profile_match,omitempty"`
	Signature        string           `json:"signature,omitempty"`
	Attestation      *attestationWire `json:"attestation,omitempty"`
}

type noMatchResponse struct {
	Found   bool   `json:"found"`
	NoMatch bool   `json:"no_match"`
	Reason  string `json:"reason"`
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) agentic.ToolResult {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}
}
