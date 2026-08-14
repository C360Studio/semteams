package sandboxmanager

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/message"
)

func TestMockRunner_UpReady(t *testing.T) {
	ref, err := MockRunner{}.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/cfg.json", nil)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if ref.ContainerID == "" {
		t.Fatalf("ContainerID empty")
	}
	if !strings.HasPrefix(ref.ImageDigest, "sha256:mock") {
		t.Fatalf("image digest not stamped: %q", ref.ImageDigest)
	}
}

func TestMockRunner_UpStable(t *testing.T) {
	a, _ := MockRunner{}.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/cfg.json", nil)
	b, _ := MockRunner{}.Up(context.Background(), "/var/lib/semteams-tenants/abc/workspace", "/cfg.json", nil)
	if a.ContainerID != b.ContainerID || a.ImageDigest != b.ImageDigest {
		t.Fatalf("MockRunner not stable across calls: %v vs %v", a, b)
	}
}

func TestMockRunner_ExecEchoes(t *testing.T) {
	res, err := MockRunner{}.Exec(context.Background(),
		ContainerRef{ContainerID: "c1", HostWorkspaceFolder: "/w", RemoteWorkspaceFolder: "/workspaces/mock-x"},
		"go version")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "go version") {
		t.Fatalf("command not echoed: %q", res.Stdout)
	}
}

func TestMockRunner_AttestationFlow(t *testing.T) {
	prof := goBackendProfile()
	req := SandboxRequirements{
		Languages:    []string{"go"},
		Verification: []VerifyProbe{{Name: "go", Command: "go version"}},
	}
	ref, _ := MockRunner{}.Up(context.Background(), "/w", "/c", nil)
	probe, _ := MockRunner{}.Exec(context.Background(), ref, "go version")
	probe.Name = "go"
	probe.ExpectExit = 0
	att := Attest(prof, req, ref.ImageDigest, ref.HostWorkspaceFolder, []ProbeResult{probe}, fixedTime())
	if !att.Ready {
		t.Fatalf("expected Ready=true with MockRunner end-to-end")
	}
	if att.Verified["go"] == "" {
		t.Fatalf("Verified[go] empty: %+v", att.Verified)
	}
}

// noopPublisher is a TriplePublisher that drops every call. Used by
// the Manager-round-trip determinism test below — we care about
// Signature/ImageDigest/Ready, not what the publisher receives.
type noopPublisher struct{}

func (noopPublisher) AddTriple(_ context.Context, _ message.Triple) error { return nil }
func (noopPublisher) AddTriplesBatch(_ context.Context, _ []message.Triple) error {
	return nil
}

func TestMockRunner_ManagerRoundTrip_StableSignature(t *testing.T) {
	// Per PR 4.4 finding M3: end-to-end Manager.Request through
	// MockRunner must produce stable Signature + ImageDigest for
	// identical requirements. Guards against silent drift in
	// Manager.workspaceFor / SignatureFor that would only surface
	// in production cache-key mismatches.
	cat, err := NewCatalog([]Profile{goBackendProfile()})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	mgr := New(ManagerConfig{
		Catalog:   cat,
		Runner:    MockRunner{},
		Publisher: noopPublisher{},
		Now:       fixedTime,
	})
	req := SandboxRequirements{
		Languages: []string{"go"},
		Tools:     []string{"task"},
		Verification: []VerifyProbe{
			{Name: "go", Command: "go version"},
		},
	}
	a, err := mgr.Request(context.Background(), "", req)
	if err != nil {
		t.Fatalf("Request a: %v", err)
	}
	b, err := mgr.Request(context.Background(), "", req)
	if err != nil {
		t.Fatalf("Request b: %v", err)
	}
	if a.Signature != b.Signature {
		t.Fatalf("identical requirements produced different signatures: %s vs %s", a.Signature, b.Signature)
	}
	if a.ImageDigest != b.ImageDigest {
		t.Fatalf("identical requirements produced different image digests: %s vs %s", a.ImageDigest, b.ImageDigest)
	}
	if !a.Ready || !b.Ready {
		t.Fatalf("MockRunner Manager round-trip did not reach Ready (a.Ready=%v, b.Ready=%v)", a.Ready, b.Ready)
	}
	if a.Verified["go"] == "" {
		t.Fatalf("Verified[go] missing on round-trip: %+v", a.Verified)
	}
}

// CreateEntityWithTriples satisfies beta.159's widened TriplePublisher.
func (noopPublisher) CreateEntityWithTriples(context.Context, string, message.Type, []message.Triple) error {
	return nil
}
