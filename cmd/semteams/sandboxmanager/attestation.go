package sandboxmanager

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semstreams/message"
)

// Predicate constants stamped on `sandbox.attestation.*`. The
// namespace is operational state, not domain truth: triples age
// out per profile TTL and live on the calling chain entity.
//
// Coordinator persona prose substitutes
// `$entity.triple.sandbox.attestation.{ready,profile,verified.*}`
// when routing on a request_sandbox result. Adding a new predicate
// is a contract-widening change: declare here, reference in rule +
// persona, update the docs cross-link.
//
// Reasons (degraded / admission_pending / admission_denied) stamp
// as ARRAYS of strings (one triple, Object = []string), not as a
// joined string. PR 4.2 finding M5: a reason fragment containing
// the join separator would silently split into multiple reasons on
// the read side. Native typed-object triples avoid that class.
const (
	PredicateAttestationProfile          = "sandbox.attestation.profile"
	PredicateAttestationImageDigest      = "sandbox.attestation.image_digest"
	PredicateAttestationReady            = "sandbox.attestation.ready"
	PredicateAttestationAttestedAt       = "sandbox.attestation.attested_at"
	PredicateAttestationDegraded         = "sandbox.attestation.degraded"
	PredicateAttestationDegradedReasons  = "sandbox.attestation.degraded_reasons"
	PredicateAttestationFailedProbes     = "sandbox.attestation.failed_probes"
	PredicateAttestationVerifiedPrefix   = "sandbox.attestation.verified."
	PredicateAttestationAdmission        = "sandbox.attestation.admission"
	PredicateAttestationAdmissionReasons = "sandbox.attestation.admission_reasons"
	PredicateAttestationTerminal         = "sandbox.attestation.terminal"
	PredicateAttestationTerminalReason   = "sandbox.attestation.terminal_reason"
	PredicateAttestationRequirementsHash = "sandbox.attestation.requirements_hash"
	PredicateAttestationSignature        = "sandbox.attestation.signature"
	PredicateAttestationTTL              = "sandbox.attestation.ttl_seconds"
)

// AttestationSource tags triples this package publishes. Distinct
// from sandboxfleet's `sandbox-fleet-registry` so operators grepping
// the graph can see what stamped a sandbox.* predicate. Exported so
// PR 4.2's tool consumers can pattern-match on the source without
// string-literal drift (PR 4.1 finding N1).
const AttestationSource = "sandbox-manager-attestation"

// ProbeResult captures the outcome of one VerifyProbe execution.
// Stdout is the raw probe output; the manager trims and truncates
// before stamping (long stdouts blow up the graph; the full output
// stays in the manager's structured logs for ops). ExpectExit is
// copied from the originating VerifyProbe by the manager so OK()
// can compare against the contract authoritatively (per PR 4.1
// finding M2 — making ExpectExit explicit on the result avoids
// runProbes having to synthesize an Err for non-zero-but-expected
// exits, which was masking probe semantics).
type ProbeResult struct {
	Name       string
	Command    string
	ExitCode   int
	ExpectExit int
	Stdout     string
	Stderr     string
	Err        error
}

// OK reports whether the probe passed: no execution error AND the
// exit code matches the probe's declared ExpectExit (zero by
// default, but probes that pass on non-zero exit work too).
func (p ProbeResult) OK() bool {
	return p.Err == nil && p.ExitCode == p.ExpectExit
}

// Attestation is the operational-state record the Coordinator
// routes on. Wire-form is the JSON marshalling of this struct +
// the graph triples returned by Triples().
type Attestation struct {
	// ProfileID is the "name@version" the requirements matched. Stable
	// across attestations for a profile that hasn't been bumped.
	ProfileID string

	// ImageDigest is the materialized container's image content
	// hash (sha256:...). Empty when AdmissionOutcome != Admitted
	// (no container was brought up).
	ImageDigest string

	// Ready reports whether all preflight probes passed. Routes on
	// this for the happy path.
	Ready bool

	// AttestedAt is when the manager finished stamping. Freshness
	// is computed against this + the profile's TTL.
	AttestedAt time.Time

	// Degraded is true when some probes passed but at least one
	// failed. The Coordinator may route to a remediation persona
	// (probe-specific) when Degraded=true.
	Degraded bool

	// DegradedReasons enumerates the human-readable reasons (one per
	// failed probe). Empty when not degraded.
	DegradedReasons []string

	// FailedProbes is the list of probe names that failed. Used by
	// the rule pack to identify which capabilities to remediate.
	FailedProbes []string

	// Verified maps probe name → verified value (stdout trimmed +
	// truncated). Only populated for probes that passed. The
	// Coordinator may key on specific entries (e.g.
	// `Verified["go"] = "go version go1.25.4 linux/amd64"`).
	Verified map[string]string

	// AdmissionOutcome captures the admission stage result. Wire-form
	// is the lowercased enum string; routes when != Admitted.
	AdmissionOutcome AdmissionOutcome

	// AdmissionReasons is the union of pending + denied admission
	// reasons in stable order. Empty when Admitted.
	AdmissionReasons []string

	// Terminal is true when AdmissionOutcome=Denied OR the manager
	// hit an internal error that can't be retried. Coordinator
	// surfaces the failure to user; no further action available.
	Terminal bool

	// TerminalReason is the human-readable message stamped when
	// Terminal=true.
	TerminalReason string

	// RequirementsHash is SandboxRequirements.Hash(). One half of
	// the (profile, requirements_hash) cache key.
	RequirementsHash string

	// Signature is the composite content-address:
	// SHA-256(profile_id + ":" + requirements_hash). Used by
	// query_sandbox_attestation to look up an existing attestation
	// for the same logical request.
	Signature string

	// TTL is the freshness window the manager assigned. After this
	// duration, the attestation is stale and re-attestation kicks
	// off on next request_sandbox.
	TTL time.Duration
}

// SignatureFor computes the composite content-address for a
// (Profile, SandboxRequirements) pair. Stable across runs given
// equivalent inputs (Profile.ID() + Normalize'd requirements hash).
// Used both at Attest time (stamp the signature triple) and by
// query_sandbox_attestation in PR 4.2 (look up by signature).
func SignatureFor(profile Profile, req SandboxRequirements) string {
	composite := profile.ID() + ":" + req.Hash()
	sum := sha256.Sum256([]byte(composite))
	return hex.EncodeToString(sum[:])
}

// IsFresh reports whether the attestation is fresh enough for the
// caller to reuse without re-running up + probes. Stale = AttestedAt
// + TTL < now. Zero AttestedAt is treated as stale (defensive — an
// uninitialized struct should not pass freshness).
func (a Attestation) IsFresh(now time.Time) bool {
	if a.AttestedAt.IsZero() {
		return false
	}
	ttl := a.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return now.Before(a.AttestedAt.Add(ttl))
}

// Attest renders a SandboxRequirements + matched Profile + image
// digest + probe results into a final Attestation. Ready is true
// iff all probes passed; Degraded is true iff some passed and some
// failed; failure mode is all-fail (Ready=false, Degraded=false —
// nothing salvageable). attestedAt is injected so callers can
// fix time in tests; production passes time.Now().
//
// Probe order in DegradedReasons + FailedProbes is sorted by probe
// name for stable wire form. Verified map keys mirror probe names
// case-insensitively (lowercased).
func Attest(profile Profile, req SandboxRequirements, imageDigest string, probes []ProbeResult, attestedAt time.Time) Attestation {
	att := Attestation{
		ProfileID:        profile.ID(),
		ImageDigest:      imageDigest,
		AttestedAt:       attestedAt.UTC(),
		AdmissionOutcome: AdmissionAdmitted,
		RequirementsHash: req.Hash(),
		Signature:        SignatureFor(profile, req),
		TTL:              profile.ResolvedTTL(),
		Verified:         make(map[string]string, len(probes)),
	}

	var failed, passed int
	for _, p := range probes {
		name := strings.ToLower(strings.TrimSpace(p.Name))
		if p.OK() {
			att.Verified[name] = truncateVerified(p.Stdout)
			passed++
		} else {
			att.FailedProbes = append(att.FailedProbes, name)
			reason := failureReason(p)
			att.DegradedReasons = append(att.DegradedReasons, fmt.Sprintf("%s: %s", name, reason))
			failed++
		}
	}
	sort.Strings(att.FailedProbes)
	sort.Strings(att.DegradedReasons)

	switch {
	case failed == 0 && passed > 0:
		att.Ready = true
	case failed == 0 && passed == 0:
		// No probes declared. Treat as ready — a Coordinator can
		// route on "container up" alone when the requirement chose
		// not to gate on capabilities.
		att.Ready = true
	case failed > 0 && passed > 0:
		att.Degraded = true
	default:
		// all-fail; not ready, not degraded — Coordinator routes to
		// terminal / remediation.
		att.Terminal = true
		att.TerminalReason = "all preflight probes failed"
	}

	return att
}

// AttestAdmissionFailure produces an Attestation that captures an
// admission denial or pending state — no container was brought up,
// no probes ran. Used by Manager.Request when admission gates before
// the up/probe stage. Ready=false; Outcome reflects the admission
// result; Terminal=true when AdmissionDenied.
func AttestAdmissionFailure(profile Profile, req SandboxRequirements, res AdmissionResult, attestedAt time.Time) Attestation {
	att := Attestation{
		ProfileID:        profile.ID(),
		AttestedAt:       attestedAt.UTC(),
		AdmissionOutcome: res.Outcome,
		AdmissionReasons: res.Reasons(),
		RequirementsHash: req.Hash(),
		Signature:        SignatureFor(profile, req),
		TTL:              profile.ResolvedTTL(),
		Verified:         map[string]string{},
	}
	if res.Outcome == AdmissionDenied {
		att.Terminal = true
		att.TerminalReason = "admission denied: " + strings.Join(res.DeniedReasons, "; ")
	}
	return att
}

// AttestTerminalError is the catch-all for manager-internal failures
// (devcontainer up errored, probe runner died, etc.). Terminal=true;
// AdmissionOutcome inherits the prior state (admitted, since we made
// it past admission). Used by Manager.Request.
func AttestTerminalError(profile Profile, req SandboxRequirements, reason string, attestedAt time.Time) Attestation {
	return Attestation{
		ProfileID:        profile.ID(),
		AttestedAt:       attestedAt.UTC(),
		AdmissionOutcome: AdmissionAdmitted,
		RequirementsHash: req.Hash(),
		Signature:        SignatureFor(profile, req),
		TTL:              profile.ResolvedTTL(),
		Verified:         map[string]string{},
		Terminal:         true,
		TerminalReason:   reason,
	}
}

// Triples renders the Attestation into graph triples stamped on
// subjectEntityID. Returns an empty slice when subjectEntityID is
// empty (manager has no chain entity to stamp on — e.g. in unit
// tests). Verified.<name> entries become one triple each so
// Coordinator can substitute `$entity.triple.sandbox.attestation.
// verified.<name>` for the value.
func (a Attestation) Triples(subjectEntityID string) []message.Triple {
	if subjectEntityID == "" {
		return nil
	}
	now := a.AttestedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	out := []message.Triple{
		baseAttTriple(subjectEntityID, PredicateAttestationProfile, a.ProfileID, now),
		baseAttTriple(subjectEntityID, PredicateAttestationReady, a.Ready, now),
		baseAttTriple(subjectEntityID, PredicateAttestationAttestedAt, a.AttestedAt.Format(time.RFC3339Nano), now),
		baseAttTriple(subjectEntityID, PredicateAttestationDegraded, a.Degraded, now),
		baseAttTriple(subjectEntityID, PredicateAttestationAdmission, string(a.AdmissionOutcome), now),
		baseAttTriple(subjectEntityID, PredicateAttestationTerminal, a.Terminal, now),
		baseAttTriple(subjectEntityID, PredicateAttestationRequirementsHash, a.RequirementsHash, now),
		baseAttTriple(subjectEntityID, PredicateAttestationSignature, a.Signature, now),
		baseAttTriple(subjectEntityID, PredicateAttestationTTL, int64(a.TTL.Seconds()), now),
	}

	if a.ImageDigest != "" {
		out = append(out, baseAttTriple(subjectEntityID, PredicateAttestationImageDigest, a.ImageDigest, now))
	}
	if a.TerminalReason != "" {
		out = append(out, baseAttTriple(subjectEntityID, PredicateAttestationTerminalReason, a.TerminalReason, now))
	}
	// Per PR 4.2 finding M5: stamp reason slices as native
	// []string Objects (NATS triple ingest JSON-encodes the Object;
	// the entity reader returns them as []any on the read side).
	// Per-reason indexing avoids the separator-collision class
	// (smoke #12 lesson — string-joined surfaces silently split
	// when a fragment contains the separator).
	if len(a.DegradedReasons) > 0 {
		out = append(out, baseAttTriple(subjectEntityID, PredicateAttestationDegradedReasons, append([]string(nil), a.DegradedReasons...), now))
	}
	if len(a.FailedProbes) > 0 {
		out = append(out, baseAttTriple(subjectEntityID, PredicateAttestationFailedProbes, append([]string(nil), a.FailedProbes...), now))
	}
	if len(a.AdmissionReasons) > 0 {
		out = append(out, baseAttTriple(subjectEntityID, PredicateAttestationAdmissionReasons, append([]string(nil), a.AdmissionReasons...), now))
	}

	// Per-capability verified triples: sorted by name for stable
	// graph order, which helps audit diffs. Skip entries with empty
	// values — Coordinator substitution routes on
	// $entity.triple.sandbox.attestation.verified.<name>; a
	// silently-empty triple looks "present" to a presence check but
	// resolves to "" in the spawn prompt (smoke #12 lesson — the
	// substitution-silently-empty failure shape is the one to
	// fail-shut on). Per PR 4.1 finding M3.
	verifiedKeys := make([]string, 0, len(a.Verified))
	for k := range a.Verified {
		verifiedKeys = append(verifiedKeys, k)
	}
	sort.Strings(verifiedKeys)
	for _, k := range verifiedKeys {
		v := a.Verified[k]
		if v == "" {
			continue
		}
		out = append(out, baseAttTriple(subjectEntityID, PredicateAttestationVerifiedPrefix+k, v, now))
	}
	return out
}

func baseAttTriple(subject, predicate string, object any, now time.Time) message.Triple {
	return message.Triple{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Source:     AttestationSource,
		Timestamp:  now,
		Confidence: 1.0,
	}
}

// truncateVerified bounds the Verified[name] value at a few hundred
// bytes. Probe stdouts can be huge (think `go env` or `task --list`);
// the graph triple stays modest, and operators can pull the full
// output from the manager's structured logs if they need it.
const verifiedMaxBytes = 512

func truncateVerified(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= verifiedMaxBytes {
		return s
	}
	return s[:verifiedMaxBytes] + "…(truncated)"
}

func failureReason(p ProbeResult) string {
	if p.Err != nil {
		return p.Err.Error()
	}
	return fmt.Sprintf("exit %d", p.ExitCode)
}
