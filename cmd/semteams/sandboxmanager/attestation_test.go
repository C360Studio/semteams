package sandboxmanager

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2026, 5, 31, 14, 0, 0, 0, time.UTC)
}

// testHostWorkspaceFolder is the canonical wsf shape Manager.workspaceFor
// produces (per-signature subdir under TenantRoot). Tests share it so a
// future path-layout change is a one-line edit, not a search-and-replace.
const testHostWorkspaceFolder = "/var/lib/semteams-tenants/abc/workspace"

func TestAttest_AllProbesPass_ReadyTrue(t *testing.T) {
	prof := goBackendProfile()
	prof.TTL = 6 * time.Hour
	req := SandboxRequirements{
		Languages: []string{"go"},
		Verification: []VerifyProbe{
			{Name: "go", Command: "go version"},
			{Name: "task", Command: "task --list"},
		},
	}
	probes := []ProbeResult{
		{Name: "go", Stdout: "go version go1.25.4 linux/amd64", ExitCode: 0},
		{Name: "task", Stdout: "task tasks:\n  build", ExitCode: 0},
	}
	a := Attest(prof, req, "sha256:abc123", testHostWorkspaceFolder, probes, fixedTime())
	if !a.Ready {
		t.Fatalf("expected Ready=true, got false; %+v", a)
	}
	if a.Degraded {
		t.Fatalf("expected Degraded=false")
	}
	if got := a.Verified["go"]; !strings.Contains(got, "go1.25.4") {
		t.Fatalf("Verified[go] missing version: %q", got)
	}
	if a.TTL != 6*time.Hour {
		t.Fatalf("TTL not inherited from profile: got %v", a.TTL)
	}
	if a.ProfileID != "go-backend@v1" {
		t.Fatalf("ProfileID wrong: %q", a.ProfileID)
	}
}

func TestAttest_SomeFailDegradedTrue(t *testing.T) {
	prof := goBackendProfile()
	req := SandboxRequirements{}
	probes := []ProbeResult{
		{Name: "go", Stdout: "go version go1.25.4", ExitCode: 0},
		{Name: "task", Stdout: "", ExitCode: 127, Stderr: "task: command not found"},
	}
	a := Attest(prof, req, "sha256:abc", "", probes, fixedTime())
	if a.Ready {
		t.Fatalf("expected Ready=false on degraded")
	}
	if !a.Degraded {
		t.Fatalf("expected Degraded=true")
	}
	if len(a.FailedProbes) != 1 || a.FailedProbes[0] != "task" {
		t.Fatalf("FailedProbes wrong: %v", a.FailedProbes)
	}
	if len(a.DegradedReasons) != 1 || !strings.Contains(a.DegradedReasons[0], "exit 127") {
		t.Fatalf("DegradedReasons wrong: %v", a.DegradedReasons)
	}
}

func TestAttest_AllFail_Terminal(t *testing.T) {
	prof := goBackendProfile()
	probes := []ProbeResult{
		{Name: "go", ExitCode: 127, Stderr: "not found"},
		{Name: "task", ExitCode: 127, Stderr: "not found"},
	}
	a := Attest(prof, SandboxRequirements{}, "sha256:x", "", probes, fixedTime())
	if a.Ready {
		t.Fatalf("expected Ready=false")
	}
	if a.Degraded {
		t.Fatalf("expected Degraded=false when all fail (terminal class)")
	}
	if !a.Terminal {
		t.Fatalf("expected Terminal=true")
	}
}

func TestAttest_ProbeErrCountsAsFailure(t *testing.T) {
	probes := []ProbeResult{
		{Name: "go", Err: errors.New("exec timeout")},
		{Name: "task", ExitCode: 0},
	}
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:x", "", probes, fixedTime())
	if !a.Degraded {
		t.Fatalf("expected Degraded=true (mix err + ok)")
	}
	if !strings.Contains(a.DegradedReasons[0], "exec timeout") {
		t.Fatalf("DegradedReasons did not include err message: %v", a.DegradedReasons)
	}
}

func TestAttest_EmptyProbesReady(t *testing.T) {
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:x", "", nil, fixedTime())
	if !a.Ready {
		t.Fatalf("expected Ready=true on empty probe set (container up, no capability gates)")
	}
}

func TestAttest_VerifiedTruncates(t *testing.T) {
	huge := strings.Repeat("a", 1024)
	probes := []ProbeResult{{Name: "noisy", Stdout: huge, ExitCode: 0}}
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:x", "", probes, fixedTime())
	got := a.Verified["noisy"]
	if !strings.HasSuffix(got, "(truncated)") {
		t.Fatalf("verified value not truncated: len=%d", len(got))
	}
	if len(got) >= len(huge) {
		t.Fatalf("truncated value length %d >= raw length %d", len(got), len(huge))
	}
}

func TestAttest_TriplesSkipEmptyVerified(t *testing.T) {
	// Per PR 4.1 finding M3: a probe that passes with empty stdout
	// must NOT stamp a sandbox.attestation.verified.<name> triple,
	// because Coordinator substitution would resolve to "" silently.
	probes := []ProbeResult{
		{Name: "silent", Stdout: "", ExitCode: 0},
		{Name: "loud", Stdout: "ok", ExitCode: 0},
	}
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:x", "", probes, fixedTime())
	triples := a.Triples("c360.platform1.agent.chain.execution.c1")
	for _, tr := range triples {
		if tr.Predicate == PredicateAttestationVerifiedPrefix+"silent" {
			t.Fatalf("verified.silent stamped despite empty stdout")
		}
	}
	// loud must still be stamped
	found := false
	for _, tr := range triples {
		if tr.Predicate == PredicateAttestationVerifiedPrefix+"loud" {
			found = true
		}
	}
	if !found {
		t.Fatalf("verified.loud missing")
	}
}

func TestProbeResultOK_HonorsExpectExit(t *testing.T) {
	// Per PR 4.1 finding M2: a probe that ExpectExit=1 and exits 1
	// must report OK=true.
	p := ProbeResult{ExitCode: 1, ExpectExit: 1}
	if !p.OK() {
		t.Fatalf("expected OK on exit==expect")
	}
	p2 := ProbeResult{ExitCode: 0, ExpectExit: 1}
	if p2.OK() {
		t.Fatalf("expected !OK when exit differs from expect")
	}
}

func TestAttest_VerifiedKeyLowercased(t *testing.T) {
	probes := []ProbeResult{{Name: "GO_Version", Stdout: "go1.25", ExitCode: 0}}
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:x", "", probes, fixedTime())
	if _, ok := a.Verified["go_version"]; !ok {
		t.Fatalf("Verified key not lowercased: %v", a.Verified)
	}
}

func TestSignatureFor_Stable(t *testing.T) {
	prof := Profile{Name: "go-backend", Version: "v1"}
	req := SandboxRequirements{Languages: []string{"go", "node"}}
	a := SignatureFor(prof, req)
	b := SignatureFor(prof, req)
	if a != b {
		t.Fatalf("signature drift across runs: %s vs %s", a, b)
	}
	// Different profile → different signature
	prof2 := Profile{Name: "svelte-ui", Version: "v1"}
	if SignatureFor(prof2, req) == a {
		t.Fatalf("profile change did not change signature")
	}
	// Different requirements → different signature
	req2 := SandboxRequirements{Languages: []string{"rust"}}
	if SignatureFor(prof, req2) == a {
		t.Fatalf("requirements change did not change signature")
	}
}

func TestAttestation_IsFresh(t *testing.T) {
	a := Attestation{AttestedAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC), TTL: time.Hour}
	if !a.IsFresh(time.Date(2026, 5, 31, 12, 30, 0, 0, time.UTC)) {
		t.Fatalf("expected fresh at +30min")
	}
	if a.IsFresh(time.Date(2026, 5, 31, 13, 30, 0, 0, time.UTC)) {
		t.Fatalf("expected stale at +1h30m")
	}
	zero := Attestation{TTL: time.Hour}
	if zero.IsFresh(time.Now()) {
		t.Fatalf("zero AttestedAt should be stale")
	}
	defaultTTL := Attestation{AttestedAt: time.Now().UTC()}
	if !defaultTTL.IsFresh(time.Now()) {
		t.Fatalf("zero TTL should default to DefaultTTL")
	}
}

func TestAttest_TriplesShape(t *testing.T) {
	probes := []ProbeResult{
		{Name: "go", Stdout: "go version go1.25.4", ExitCode: 0},
		{Name: "task", Stdout: "", ExitCode: 127, Stderr: "task not found"},
	}
	a := Attest(goBackendProfile(), SandboxRequirements{Languages: []string{"go"}}, "sha256:img", testHostWorkspaceFolder, probes, fixedTime())
	triples := a.Triples("c360.platform1.agent.chain.execution.testchain")

	if len(triples) == 0 {
		t.Fatalf("Triples returned empty for non-empty entity id")
	}
	want := map[string]bool{
		PredicateAttestationProfile:               true,
		PredicateAttestationReady:                 true,
		PredicateAttestationAttestedAt:            true,
		PredicateAttestationDegraded:              true,
		PredicateAttestationAdmission:             true,
		PredicateAttestationTerminal:              true,
		PredicateAttestationRequirementsHash:      true,
		PredicateAttestationSignature:             true,
		PredicateAttestationTTL:                   true,
		PredicateAttestationImageDigest:           true,
		PredicateAttestationHostWorkspaceFolder:   true,
		PredicateAttestationDegradedReasons:       true,
		PredicateAttestationFailedProbes:          true,
		PredicateAttestationVerifiedPrefix + "go": true,
	}
	got := make(map[string]bool, len(triples))
	for _, tr := range triples {
		got[tr.Predicate] = true
		if tr.Subject != "c360.platform1.agent.chain.execution.testchain" {
			t.Errorf("subject drift: %q", tr.Subject)
		}
		if tr.Source != "sandbox-manager-attestation" {
			t.Errorf("source not tagged: %q", tr.Source)
		}
		// PR 4.2 finding M5: reason / failed-probe slices stamp as
		// native []string Objects, not joined strings.
		switch tr.Predicate {
		case PredicateAttestationDegradedReasons,
			PredicateAttestationFailedProbes,
			PredicateAttestationAdmissionReasons:
			if _, ok := tr.Object.([]string); !ok {
				t.Errorf("predicate %s: expected []string Object, got %T", tr.Predicate, tr.Object)
			}
		}
	}
	for p := range want {
		if !got[p] {
			t.Errorf("expected predicate %q missing", p)
		}
	}
}

func TestAttest_TriplesEmptySubjectReturnsNil(t *testing.T) {
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:x", "", nil, fixedTime())
	if got := a.Triples(""); got != nil {
		t.Fatalf("Triples on empty subject should be nil, got %d", len(got))
	}
}

func TestAttest_TriplesStampHostWorkspaceFolder(t *testing.T) {
	// Chain-scoped tools (attestation-aware bash routing) read this
	// triple to route subsequent commands INTO the per-tenant container
	// via `devcontainer exec --workspace-folder <wsf>`. Drift in either
	// the predicate name or the value here breaks that routing
	// silently. Lock predicate + value + source at the boundary.
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:img", testHostWorkspaceFolder, nil, fixedTime())
	triples := a.Triples("c360.platform1.agent.chain.execution.testchain")
	var found bool
	for _, tr := range triples {
		if tr.Predicate != PredicateAttestationHostWorkspaceFolder {
			continue
		}
		found = true
		got, ok := tr.Object.(string)
		if !ok {
			t.Fatalf("host_workspace_folder Object expected string, got %T", tr.Object)
		}
		if got != testHostWorkspaceFolder {
			t.Fatalf("host_workspace_folder value drift: got %q want %q", got, testHostWorkspaceFolder)
		}
		if tr.Source != AttestationSource {
			t.Fatalf("host_workspace_folder source drift: got %q want %q — operators grep on Source", tr.Source, AttestationSource)
		}
	}
	if !found {
		t.Fatalf("PredicateAttestationHostWorkspaceFolder not stamped — chain-scoped bash routing has nothing to read")
	}
}

func TestAttest_TriplesOmitHostWorkspaceFolderWhenEmpty(t *testing.T) {
	// AdmissionDenied / pre-up failure paths never get a host wsf.
	// Stamping an empty string would let chain-scoped bash routing
	// substitute "" and shell `devcontainer exec --workspace-folder ''`
	// — the same opaque "config not found" failure mode the wsf split
	// (ce4f07b) fixed in the runner. Omit instead.
	a := Attest(goBackendProfile(), SandboxRequirements{}, "sha256:img", "", nil, fixedTime())
	triples := a.Triples("c360.platform1.agent.chain.execution.testchain")
	for _, tr := range triples {
		if tr.Predicate == PredicateAttestationHostWorkspaceFolder {
			t.Fatalf("empty host_workspace_folder must NOT be stamped; routing would substitute '' and fail opaquely")
		}
	}
}

func TestAttestAdmissionFailure_DeniedTerminal(t *testing.T) {
	res := AdmissionResult{Outcome: AdmissionDenied, DeniedReasons: []string{"secret missing"}}
	a := AttestAdmissionFailure(goBackendProfile(), SandboxRequirements{}, res, fixedTime())
	if !a.Terminal {
		t.Fatalf("AdmissionDenied should produce Terminal=true")
	}
	if a.AdmissionOutcome != AdmissionDenied {
		t.Fatalf("AdmissionOutcome not propagated: %s", a.AdmissionOutcome)
	}
	if !strings.Contains(a.TerminalReason, "secret missing") {
		t.Fatalf("TerminalReason missing denied detail: %q", a.TerminalReason)
	}
	if a.Ready {
		t.Fatalf("denied attestation should not be Ready")
	}
}

func TestAttestAdmissionFailure_PendingNotTerminal(t *testing.T) {
	res := AdmissionResult{Outcome: AdmissionPending, PendingReasons: []string{"docker-socket review"}}
	a := AttestAdmissionFailure(goBackendProfile(), SandboxRequirements{}, res, fixedTime())
	if a.Terminal {
		t.Fatalf("AdmissionPending should NOT be Terminal")
	}
	if a.AdmissionOutcome != AdmissionPending {
		t.Fatalf("expected AdmissionPending outcome")
	}
}

func TestAttestTerminalError(t *testing.T) {
	a := AttestTerminalError(goBackendProfile(), SandboxRequirements{}, "devcontainer up failed: exit 1", fixedTime())
	if !a.Terminal {
		t.Fatalf("expected Terminal=true")
	}
	if !strings.Contains(a.TerminalReason, "devcontainer up failed") {
		t.Fatalf("TerminalReason wrong: %q", a.TerminalReason)
	}
	if a.AdmissionOutcome != AdmissionAdmitted {
		t.Fatalf("expected Admission=Admitted (we made it past admission)")
	}
}

// TestNonAdmittedPaths_NeverStampHostWorkspaceFolder guards the wsf
// triple against leaking through any non-Attest construction path.
// Background: Phase A's omit-when-empty semantics on Attest are tested
// directly above, but AttestAdmissionFailure and AttestTerminalError
// build Attestations without going through Attest(...) at all — a
// future refactor that consolidated triple-emission into a shared
// helper could regress and start stamping these paths. Lock the
// invariant on all three constructors so the wsf field always reflects
// "the manager actually brought a container up" and never "the manager
// tried and failed before Up()."
func TestNonAdmittedPaths_NeverStampHostWorkspaceFolder(t *testing.T) {
	const subject = "c360.platform1.agent.chain.execution.testchain"
	cases := []struct {
		name string
		a    Attestation
	}{
		{"admission-denied", AttestAdmissionFailure(goBackendProfile(), SandboxRequirements{},
			AdmissionResult{Outcome: AdmissionDenied, DeniedReasons: []string{"secret missing"}}, fixedTime())},
		{"admission-pending", AttestAdmissionFailure(goBackendProfile(), SandboxRequirements{},
			AdmissionResult{Outcome: AdmissionPending, PendingReasons: []string{"docker-socket review"}}, fixedTime())},
		{"terminal-error", AttestTerminalError(goBackendProfile(), SandboxRequirements{}, "devcontainer up failed: exit 1", fixedTime())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.a.HostWorkspaceFolder != "" {
				t.Fatalf("%s constructed Attestation with non-empty HostWorkspaceFolder=%q — the field must reflect post-Up state only",
					tc.name, tc.a.HostWorkspaceFolder)
			}
			for _, tr := range tc.a.Triples(subject) {
				if tr.Predicate == PredicateAttestationHostWorkspaceFolder {
					t.Fatalf("%s leaked sandbox.attestation.host_workspace_folder=%v — chain routing would substitute a stale path",
						tc.name, tr.Object)
				}
			}
		})
	}
}
