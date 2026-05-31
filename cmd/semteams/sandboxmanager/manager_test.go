package sandboxmanager

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
)

type fakeRunner struct {
	upFn   func(ctx context.Context, workspaceFolder, configPath string, env map[string]string) (ContainerRef, error)
	execFn func(ctx context.Context, ref ContainerRef, command string) (ProbeResult, error)
}

func (f *fakeRunner) Up(ctx context.Context, wsf, cfg string, env map[string]string) (ContainerRef, error) {
	if f.upFn != nil {
		return f.upFn(ctx, wsf, cfg, env)
	}
	return ContainerRef{ContainerID: "c1", ImageDigest: "sha256:fake", RemoteWorkspaceFolder: wsf}, nil
}

func (f *fakeRunner) Exec(ctx context.Context, ref ContainerRef, command string) (ProbeResult, error) {
	if f.execFn != nil {
		return f.execFn(ctx, ref, command)
	}
	return ProbeResult{ExitCode: 0, Stdout: "ok"}, nil
}

type fakePublisher struct {
	mu      sync.Mutex
	batches [][]message.Triple
	err     error
}

func (p *fakePublisher) AddTriple(_ context.Context, t message.Triple) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.batches = append(p.batches, []message.Triple{t})
	return p.err
}

func (p *fakePublisher) AddTriplesBatch(_ context.Context, ts []message.Triple) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	batch := make([]message.Triple, len(ts))
	copy(batch, ts)
	p.batches = append(p.batches, batch)
	return nil
}

func newTestManager(t *testing.T, runner Runner, publisher *fakePublisher, policy AdmissionPolicy) *Manager {
	t.Helper()
	return New(ManagerConfig{
		Catalog:   testCatalog(t),
		Policy:    policy,
		Runner:    runner,
		Publisher: publisher,
		Now:       fixedTime,
	})
}

func TestManagerRequest_HappyPath_AdmitsAndStamps(t *testing.T) {
	runner := &fakeRunner{
		execFn: func(_ context.Context, _ ContainerRef, cmd string) (ProbeResult, error) {
			switch cmd {
			case "go version":
				return ProbeResult{Stdout: "go version go1.25.4 linux/amd64", ExitCode: 0}, nil
			case "task --list":
				return ProbeResult{Stdout: "task tasks", ExitCode: 0}, nil
			}
			return ProbeResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	pub := &fakePublisher{}
	m := newTestManager(t, runner, pub, AdmissionPolicy{})
	req := SandboxRequirements{
		Languages: []string{"go"},
		Tools:     []string{"task"},
		Verification: []VerifyProbe{
			{Name: "go", Command: "go version"},
			{Name: "task", Command: "task --list"},
		},
	}
	att, err := m.Request(context.Background(), "c360.platform1.agent.chain.execution.chain1", req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if !att.Ready {
		t.Fatalf("expected Ready=true, got %+v", att)
	}
	if att.ProfileID != "go-backend@v1" {
		t.Fatalf("matched wrong profile: %q", att.ProfileID)
	}
	if att.ImageDigest != "sha256:fake" {
		t.Fatalf("ImageDigest not propagated: %q", att.ImageDigest)
	}
	if len(pub.batches) != 1 {
		t.Fatalf("expected 1 stamp batch, got %d", len(pub.batches))
	}
	if len(pub.batches[0]) == 0 {
		t.Fatalf("batch empty")
	}
}

func TestManagerRequest_AdmissionPending_NoUp(t *testing.T) {
	upCalled := false
	runner := &fakeRunner{
		upFn: func(_ context.Context, _, _ string, _ map[string]string) (ContainerRef, error) {
			upCalled = true
			return ContainerRef{}, nil
		},
	}
	pub := &fakePublisher{}
	m := newTestManager(t, runner, pub, AdmissionPolicy{})
	req := SandboxRequirements{
		Languages:  []string{"go", "node"},
		Privileges: []Privilege{PrivilegeDockerSocket},
	}
	att, err := m.Request(context.Background(), "c360.platform1.agent.chain.execution.c1", req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if att.AdmissionOutcome != AdmissionPending {
		t.Fatalf("expected AdmissionPending, got %s", att.AdmissionOutcome)
	}
	if upCalled {
		t.Fatalf("Up should not have been called on admission_pending")
	}
	if att.Ready {
		t.Fatalf("pending should not be Ready")
	}
}

func TestManagerRequest_AdmissionDenied_Terminal(t *testing.T) {
	pub := &fakePublisher{}
	m := newTestManager(t, &fakeRunner{}, pub, AdmissionPolicy{})
	req := SandboxRequirements{
		Languages: []string{"go"},
		Secrets:   []string{"NOT_PROVISIONED"},
	}
	att, _ := m.Request(context.Background(), "c360.platform1.agent.chain.execution.c1", req)
	if att.AdmissionOutcome != AdmissionDenied {
		t.Fatalf("expected denied, got %s", att.AdmissionOutcome)
	}
	if !att.Terminal {
		t.Fatalf("denied should be Terminal")
	}
}

func TestManagerRequest_NoMatch_Terminal(t *testing.T) {
	pub := &fakePublisher{}
	m := newTestManager(t, &fakeRunner{}, pub, AdmissionPolicy{})
	req := SandboxRequirements{Languages: []string{"rust"}}
	att, _ := m.Request(context.Background(), "", req)
	if !att.Terminal {
		t.Fatalf("expected terminal on no-match")
	}
	if att.ProfileID != "unmatched@v0" {
		t.Fatalf("expected unmatched sentinel profile, got %q", att.ProfileID)
	}
}

func TestManagerRequest_UpFails_Terminal(t *testing.T) {
	runner := &fakeRunner{
		upFn: func(_ context.Context, _, _ string, _ map[string]string) (ContainerRef, error) {
			return ContainerRef{}, errors.New("image pull denied")
		},
	}
	pub := &fakePublisher{}
	m := newTestManager(t, runner, pub, AdmissionPolicy{})
	req := SandboxRequirements{Languages: []string{"go"}}
	att, _ := m.Request(context.Background(), "c360.platform1.agent.chain.execution.c1", req)
	if !att.Terminal {
		t.Fatalf("expected terminal on Up failure")
	}
	if att.AdmissionOutcome != AdmissionAdmitted {
		t.Fatalf("Up failure should preserve admitted outcome")
	}
}

func TestManagerRequest_ProbeFailDegrades(t *testing.T) {
	runner := &fakeRunner{
		execFn: func(_ context.Context, _ ContainerRef, cmd string) (ProbeResult, error) {
			if cmd == "go version" {
				return ProbeResult{ExitCode: 0, Stdout: "go1.25"}, nil
			}
			return ProbeResult{ExitCode: 127, Stderr: "not found"}, nil
		},
	}
	pub := &fakePublisher{}
	m := newTestManager(t, runner, pub, AdmissionPolicy{})
	req := SandboxRequirements{
		Languages: []string{"go"},
		Verification: []VerifyProbe{
			{Name: "go", Command: "go version"},
			{Name: "missing", Command: "missing --version"},
		},
	}
	att, _ := m.Request(context.Background(), "c360.platform1.agent.chain.execution.c1", req)
	if att.Ready {
		t.Fatalf("expected Ready=false on degraded")
	}
	if !att.Degraded {
		t.Fatalf("expected Degraded=true")
	}
}

func TestManagerRequest_NoChainEntity_NoStamp(t *testing.T) {
	pub := &fakePublisher{}
	m := newTestManager(t, &fakeRunner{}, pub, AdmissionPolicy{})
	req := SandboxRequirements{Languages: []string{"go"}}
	_, err := m.Request(context.Background(), "", req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(pub.batches) != 0 {
		t.Fatalf("expected no stamp without chain entity, got %d batches", len(pub.batches))
	}
}

func TestManagerRequest_PublisherErr_Surfaced(t *testing.T) {
	pub := &fakePublisher{err: errors.New("nats down")}
	m := newTestManager(t, &fakeRunner{}, pub, AdmissionPolicy{})
	req := SandboxRequirements{Languages: []string{"go"}}
	_, err := m.Request(context.Background(), "c360.platform1.agent.chain.execution.c1", req)
	if err == nil {
		t.Fatalf("publisher err not surfaced")
	}
}

func TestManagerRequest_InvalidReq_TerminalNotErr(t *testing.T) {
	pub := &fakePublisher{}
	m := newTestManager(t, &fakeRunner{}, pub, AdmissionPolicy{})
	req := SandboxRequirements{Languages: []string{"GO LANG"}} // space, fails Validate
	att, err := m.Request(context.Background(), "", req)
	if err != nil {
		t.Fatalf("expected validation to surface as terminal Attestation, not error: %v", err)
	}
	if !att.Terminal {
		t.Fatalf("expected Terminal on invalid req")
	}
}

func TestManagerWorkspaceFor_Stable(t *testing.T) {
	m := New(ManagerConfig{
		Catalog:   testCatalog(t),
		Runner:    &fakeRunner{},
		Publisher: &fakePublisher{},
	})
	req := SandboxRequirements{Languages: []string{"go"}}
	prof, _ := MatchProfile(req, m.catalog)
	a := m.workspaceFor(req, prof)
	b := m.workspaceFor(req, prof)
	if a != b {
		t.Fatalf("workspaceFor not stable: %s vs %s", a, b)
	}
}

func TestManagerNew_PanicsOnMissingDeps(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on missing catalog")
		}
	}()
	New(ManagerConfig{Runner: &fakeRunner{}, Publisher: &fakePublisher{}})
}

func TestManagerNew_DefaultsApplied(t *testing.T) {
	m := New(ManagerConfig{
		Catalog:   testCatalog(t),
		Runner:    &fakeRunner{},
		Publisher: &fakePublisher{},
	})
	// PR 4.5 F6: default tenant root is the production-canonical
	// /var/lib/semteams-tenants path so docker-compose can bind it
	// symmetrically. Operators override via SEMTEAMS_TENANT_ROOT.
	if m.tenantRoot != "/var/lib/semteams-tenants" {
		t.Fatalf("tenantRoot default wrong: %q", m.tenantRoot)
	}
	if m.probeTimeout != 30*time.Second {
		t.Fatalf("probeTimeout default wrong: %v", m.probeTimeout)
	}
	if m.upTimeout != 5*time.Minute {
		t.Fatalf("upTimeout default wrong: %v", m.upTimeout)
	}
}

func TestManagerRequest_NormalizesBeforeProbe(t *testing.T) {
	// Per PR 4.1 finding C3: probe Command/Name must be normalized
	// (trimmed, lowercased) before being executed and stamped on
	// attestation. Otherwise Coordinator substitution
	// $entity.triple.sandbox.attestation.verified.<name> drifts from
	// the form the rule pack expects.
	var seenCommand string
	runner := &fakeRunner{
		execFn: func(_ context.Context, _ ContainerRef, cmd string) (ProbeResult, error) {
			seenCommand = cmd
			return ProbeResult{ExitCode: 0, Stdout: "go version go1.25"}, nil
		},
	}
	pub := &fakePublisher{}
	m := newTestManager(t, runner, pub, AdmissionPolicy{})
	req := SandboxRequirements{
		Languages: []string{"go"},
		Verification: []VerifyProbe{
			{Name: "GO", Command: "  go version  "},
		},
	}
	att, err := m.Request(context.Background(), "c360.platform1.agent.chain.execution.c1", req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if seenCommand != "go version" {
		t.Fatalf("probe command not trimmed before exec: %q", seenCommand)
	}
	if _, ok := att.Verified["go"]; !ok {
		t.Fatalf("Verified key not lowercased post-normalize: %v", att.Verified)
	}
}

func TestManagerSecretEnv_NamesOnly(t *testing.T) {
	m := New(ManagerConfig{
		Catalog:   testCatalog(t),
		Runner:    &fakeRunner{},
		Publisher: &fakePublisher{},
	})
	env := m.secretEnv(SandboxRequirements{Secrets: []string{"OPENAI_API_KEY", "GITHUB_TOKEN"}})
	if _, ok := env["OPENAI_API_KEY"]; !ok {
		t.Fatalf("expected OPENAI_API_KEY in env map: %v", env)
	}
	if env["OPENAI_API_KEY"] != "" {
		t.Fatalf("expected empty sentinel value (Runner resolves at exec), got %q", env["OPENAI_API_KEY"])
	}
}
