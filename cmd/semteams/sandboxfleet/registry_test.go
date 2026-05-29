package sandboxfleet

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
)

// fakeTriplePublisher captures published triples per subject. Indexed
// by subject (the tenant entity id) so a single test can publish
// across multiple signatures and verify per-tenant state.
type fakeTriplePublisher struct {
	mu      sync.Mutex
	triples map[string][]message.Triple
	addErr  error
}

func newFakePub() *fakeTriplePublisher {
	return &fakeTriplePublisher{triples: map[string][]message.Triple{}}
}

func (f *fakeTriplePublisher) AddTriple(_ context.Context, t message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return f.addErr
	}
	f.triples[t.Subject] = append(f.triples[t.Subject], t)
	return nil
}

func (f *fakeTriplePublisher) AddTriplesBatch(_ context.Context, ts []message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return f.addErr
	}
	for _, t := range ts {
		f.triples[t.Subject] = append(f.triples[t.Subject], t)
	}
	return nil
}

// latest returns the most recent value for a predicate (mirrors
// graph-ingest's last-wins semantics).
func (f *fakeTriplePublisher) latest(subject, predicate string) (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.triples[subject]) - 1; i >= 0; i-- {
		if f.triples[subject][i].Predicate == predicate {
			return f.triples[subject][i].Object, true
		}
	}
	return nil, false
}

// fakeReader replays the publisher's state as the read shape.
// Implements EntityTripleReader without a live graph component.
type fakeReader struct {
	pub *fakeTriplePublisher
	err error
}

func (r *fakeReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.pub.mu.Lock()
	defer r.pub.mu.Unlock()
	out := map[string]any{}
	for _, t := range r.pub.triples[entityID] {
		out[t.Predicate] = t.Object
	}
	return out, nil
}

func testSig(t *testing.T) TargetSignature {
	t.Helper()
	sig, err := Canonicalize(CanonicalizeInput{
		Command:   "task test:integration",
		BaseImage: "ubuntu:24.04",
		RepoURL:   "https://github.com/c360studio/semteams",
		RepoRef:   "main",
		Toolchain: map[string]string{"go": "1.26.0"},
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return sig
}

func TestTenantRegistry_LookupMiss(t *testing.T) {
	pub := newFakePub()
	r := NewTenantRegistry(pub, &fakeReader{pub: pub}, "c360", "ops", slog.Default())
	rec, err := r.Lookup(context.Background(), testSig(t))
	if err != nil {
		t.Fatalf("lookup miss: %v", err)
	}
	if rec != nil {
		t.Errorf("expected nil record on miss, got %+v", rec)
	}
}

func TestTenantRegistry_MarkProvisioning(t *testing.T) {
	pub := newFakePub()
	r := NewTenantRegistry(pub, &fakeReader{pub: pub}, "c360", "ops", slog.Default())
	sig := testSig(t)

	if err := r.MarkProvisioning(context.Background(), sig, "plan-hash-1"); err != nil {
		t.Fatalf("mark provisioning: %v", err)
	}

	rec, err := r.Lookup(context.Background(), sig)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if rec == nil {
		t.Fatalf("expected record after provisioning, got nil")
	}
	if rec.State != StateProvisioning {
		t.Errorf("state = %q, want %q", rec.State, StateProvisioning)
	}
	if rec.Signature != sig.Hash() {
		t.Errorf("signature drift: rec=%s want=%s", rec.Signature, sig.Hash())
	}
	if rec.PlanHash != "plan-hash-1" {
		t.Errorf("plan hash = %q, want plan-hash-1", rec.PlanHash)
	}
	// ContainerName + Image + Workspace not yet populated (Commit job).
	if rec.ContainerName != "" {
		t.Errorf("container name should be empty before commit, got %q", rec.ContainerName)
	}
}

func TestTenantRegistry_CommitTransitionsToReadyRunning(t *testing.T) {
	pub := newFakePub()
	reader := &fakeReader{pub: pub}
	r := NewTenantRegistry(pub, reader, "c360", "ops", slog.Default())
	sig := testSig(t)

	if err := r.MarkProvisioning(context.Background(), sig, "plan-hash-1"); err != nil {
		t.Fatalf("mark provisioning: %v", err)
	}
	if err := r.Commit(context.Background(), sig, sig.ContainerName(), "ubuntu:24.04", "/workspace", "plan-hash-1"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	rec, err := r.Lookup(context.Background(), sig)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if rec == nil {
		t.Fatalf("expected record after commit, got nil")
	}
	if rec.State != StateReadyRunning {
		t.Errorf("state = %q, want %q", rec.State, StateReadyRunning)
	}
	if rec.ContainerName != sig.ContainerName() {
		t.Errorf("container name = %q, want %q", rec.ContainerName, sig.ContainerName())
	}
	if rec.Image != "ubuntu:24.04" {
		t.Errorf("image = %q, want ubuntu:24.04", rec.Image)
	}
	if rec.Workspace != "/workspace" {
		t.Errorf("workspace = %q, want /workspace", rec.Workspace)
	}
	if rec.ReadyAt.IsZero() {
		t.Errorf("ready_at unset after commit")
	}
	if rec.LastUsed.IsZero() {
		t.Errorf("last_used unset after commit")
	}
}

func TestTenantRegistry_MarkState(t *testing.T) {
	pub := newFakePub()
	r := NewTenantRegistry(pub, &fakeReader{pub: pub}, "c360", "ops", slog.Default())
	sig := testSig(t)

	if err := r.Commit(context.Background(), sig, sig.ContainerName(), "ubuntu:24.04", "/workspace", "h"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := r.MarkState(context.Background(), sig, StateReadyStopped); err != nil {
		t.Fatalf("mark state: %v", err)
	}
	rec, err := r.Lookup(context.Background(), sig)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if rec.State != StateReadyStopped {
		t.Errorf("state = %q, want %q", rec.State, StateReadyStopped)
	}
}

func TestTenantRegistry_TouchLastUsed(t *testing.T) {
	pub := newFakePub()
	r := NewTenantRegistry(pub, &fakeReader{pub: pub}, "c360", "ops", slog.Default())
	sig := testSig(t)
	if err := r.Commit(context.Background(), sig, sig.ContainerName(), "ubuntu:24.04", "/workspace", "h"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	first, err := r.Lookup(context.Background(), sig)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := r.TouchLastUsed(context.Background(), sig); err != nil {
		t.Fatalf("touch: %v", err)
	}
	second, err := r.Lookup(context.Background(), sig)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !second.LastUsed.After(first.LastUsed) {
		t.Errorf("last_used not advanced: first=%s second=%s", first.LastUsed, second.LastUsed)
	}
}

func TestTenantRegistry_LookupPropagatesGraphError(t *testing.T) {
	pub := newFakePub()
	r := NewTenantRegistry(pub, &fakeReader{pub: pub, err: errors.New("boom")}, "c360", "ops", slog.Default())
	if _, err := r.Lookup(context.Background(), testSig(t)); err == nil {
		t.Errorf("expected error propagation, got nil")
	}
}

func TestRecord_IsFresh(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		rec       *Record
		planHash  string
		ttl       time.Duration
		wantFresh bool
		wantSub   string
	}{
		{
			name:      "nil record → miss",
			rec:       nil,
			wantFresh: false,
			wantSub:   "registry miss",
		},
		{
			name: "provisioning state → not ready",
			rec: &Record{
				State:    StateProvisioning,
				PlanHash: "h", ReadyAt: now,
			},
			planHash:  "h",
			ttl:       time.Hour,
			wantFresh: false,
			wantSub:   "not ready",
		},
		{
			name: "plan hash mismatch",
			rec: &Record{
				State:    StateReadyRunning,
				PlanHash: "cached", ReadyAt: now,
			},
			planHash:  "candidate",
			ttl:       time.Hour,
			wantFresh: false,
			wantSub:   "plan_hash drift",
		},
		{
			name: "stale by ttl",
			rec: &Record{
				State:    StateReadyRunning,
				PlanHash: "h",
				ReadyAt:  now.Add(-2 * time.Hour),
			},
			planHash:  "h",
			ttl:       time.Hour,
			wantFresh: false,
			wantSub:   "older than ttl",
		},
		{
			name: "fresh + ready",
			rec: &Record{
				State:    StateReadyRunning,
				PlanHash: "h", ReadyAt: now,
			},
			planHash:  "h",
			ttl:       time.Hour,
			wantFresh: true,
		},
		{
			name: "ready_stopped also fresh",
			rec: &Record{
				State:    StateReadyStopped,
				PlanHash: "h", ReadyAt: now,
			},
			planHash:  "h",
			ttl:       time.Hour,
			wantFresh: true,
		},
		{
			name: "ready_at zero",
			rec: &Record{
				State:    StateReadyRunning,
				PlanHash: "h",
			},
			planHash:  "h",
			ttl:       time.Hour,
			wantFresh: false,
			wantSub:   "ready_at missing",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fresh, reason := c.rec.IsFresh(c.planHash, c.ttl, now)
			if fresh != c.wantFresh {
				t.Errorf("fresh = %v, want %v (reason=%q)", fresh, c.wantFresh, reason)
			}
			if !c.wantFresh && c.wantSub != "" && !strings.Contains(reason, c.wantSub) {
				t.Errorf("reason %q does not contain %q", reason, c.wantSub)
			}
		})
	}
}

func TestTenantRegistry_PanicsOnNilDeps(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil publisher")
		}
	}()
	NewTenantRegistry(nil, &fakeReader{}, "c360", "ops", nil)
}

func TestParseTimeTriple(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	got := parseTimeTriple(now)
	if got.IsZero() {
		t.Errorf("expected non-zero time from %q", now)
	}
	got2 := parseTimeTriple("not-a-time")
	if !got2.IsZero() {
		t.Errorf("expected zero time from garbage, got %v", got2)
	}
	got3 := parseTimeTriple(42) // not a string
	if !got3.IsZero() {
		t.Errorf("expected zero time from non-string, got %v", got3)
	}
}
