package sandboxmanager

import (
	"strings"
	"testing"
	"time"
)

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	cat, err := NewCatalog([]Profile{
		{
			Name:              "go-backend",
			Version:           "v1",
			DevcontainerPath:  ".devcontainer/go-backend/devcontainer.json",
			Languages:         []string{"go"},
			Tools:             []string{"task", "gh", "jq"},
			AllowedNetwork:    []NetworkPolicy{NetworkRestricted, NetworkNone},
			AllowedPrivileges: nil,
			TTL:               12 * time.Hour,
		},
		{
			Name:              "svelte-ui",
			Version:           "v1",
			DevcontainerPath:  ".devcontainer/svelte-ui/devcontainer.json",
			Languages:         []string{"node"},
			Tools:             []string{"npm", "playwright"},
			AllowedNetwork:    []NetworkPolicy{NetworkRestricted, NetworkNone, NetworkPublic},
			AllowedPrivileges: nil,
		},
		{
			Name:              "full-stack-e2e",
			Version:           "v1",
			DevcontainerPath:  ".devcontainer/full-stack-e2e/devcontainer.json",
			Languages:         []string{"go", "node"},
			Tools:             []string{"task", "npm", "playwright", "docker"},
			Services:          []string{"nats"},
			AllowedNetwork:    []NetworkPolicy{NetworkRestricted, NetworkNone, NetworkPublic},
			AllowedPrivileges: []Privilege{PrivilegeDockerSocket},
		},
	})
	if err != nil {
		t.Fatalf("test catalog construct: %v", err)
	}
	return cat
}

func TestNewCatalog_RejectsDupNames(t *testing.T) {
	_, err := NewCatalog([]Profile{
		{Name: "go-backend", Version: "v1", DevcontainerPath: ".devcontainer/a/devcontainer.json", AllowedNetwork: []NetworkPolicy{NetworkRestricted}},
		{Name: "GO-BACKEND", Version: "v1", DevcontainerPath: ".devcontainer/b/devcontainer.json", AllowedNetwork: []NetworkPolicy{NetworkRestricted}},
	})
	if err == nil {
		t.Fatalf("duplicate name accepted")
	}
}

func TestNewCatalog_RequiresVersionAndPath(t *testing.T) {
	cases := []Profile{
		{Name: "x", Version: "", DevcontainerPath: "path", AllowedNetwork: []NetworkPolicy{NetworkRestricted}},
		{Name: "x", Version: "v1", DevcontainerPath: "", AllowedNetwork: []NetworkPolicy{NetworkRestricted}},
		{Name: "", Version: "v1", DevcontainerPath: "path", AllowedNetwork: []NetworkPolicy{NetworkRestricted}},
	}
	for i, p := range cases {
		if _, err := NewCatalog([]Profile{p}); err == nil {
			t.Fatalf("case %d: invalid profile accepted: %+v", i, p)
		}
	}
}

func TestNewCatalog_RequiresAllowedNetwork(t *testing.T) {
	// Per PR 4.1 finding M5: empty AllowedNetwork is operator-config
	// error, not "deny everything by default at runtime."
	_, err := NewCatalog([]Profile{
		{Name: "x", Version: "v1", DevcontainerPath: "path"},
	})
	if err == nil {
		t.Fatalf("profile with empty AllowedNetwork accepted")
	}
}

func TestProfileResolvedTTL(t *testing.T) {
	p := Profile{TTL: 0}
	if got := p.ResolvedTTL(); got != DefaultTTL {
		t.Fatalf("zero TTL not defaulted: got %v want %v", got, DefaultTTL)
	}
	p2 := Profile{TTL: 5 * time.Minute}
	if got := p2.ResolvedTTL(); got != 5*time.Minute {
		t.Fatalf("explicit TTL not honored: got %v", got)
	}
}

func TestMatchProfile_GoBackendForGoOnly(t *testing.T) {
	cat := testCatalog(t)
	req := SandboxRequirements{Languages: []string{"go"}, Tools: []string{"task"}}
	p, err := MatchProfile(req, cat)
	if err != nil {
		t.Fatalf("MatchProfile failed: %v", err)
	}
	if p.Name != "go-backend" {
		t.Fatalf("expected go-backend (snuggest fit), got %q", p.Name)
	}
}

func TestMatchProfile_FullStackForGoNode(t *testing.T) {
	cat := testCatalog(t)
	req := SandboxRequirements{
		Languages: []string{"go", "node"},
		Tools:     []string{"task", "playwright"},
	}
	p, err := MatchProfile(req, cat)
	if err != nil {
		t.Fatalf("MatchProfile failed: %v", err)
	}
	if p.Name != "full-stack-e2e" {
		t.Fatalf("expected full-stack-e2e (only profile covering both langs), got %q", p.Name)
	}
}

func TestMatchProfile_SvelteForNodeOnly(t *testing.T) {
	cat := testCatalog(t)
	req := SandboxRequirements{
		Languages: []string{"node"},
		Tools:     []string{"playwright"},
	}
	p, err := MatchProfile(req, cat)
	if err != nil {
		t.Fatalf("MatchProfile failed: %v", err)
	}
	if p.Name != "svelte-ui" {
		t.Fatalf("expected svelte-ui (snugger than full-stack), got %q", p.Name)
	}
}

func TestMatchProfile_NoMatchListsUnmet(t *testing.T) {
	cat := testCatalog(t)
	req := SandboxRequirements{
		Languages: []string{"rust"},
		Tools:     []string{"cargo"},
	}
	_, err := MatchProfile(req, cat)
	if err == nil {
		t.Fatalf("MatchProfile accepted rust workload")
	}
	if !IsNoMatch(err) {
		t.Fatalf("error is not NoMatchError: %v", err)
	}
	if !strings.Contains(err.Error(), "rust") || !strings.Contains(err.Error(), "cargo") {
		t.Fatalf("error does not mention unmet capabilities: %v", err)
	}
}

func TestMatchProfile_EmptyCatalog(t *testing.T) {
	cat, _ := NewCatalog(nil)
	_, err := MatchProfile(SandboxRequirements{Languages: []string{"go"}}, cat)
	if err != ErrEmptyCatalog {
		t.Fatalf("expected ErrEmptyCatalog, got %v", err)
	}
}

func TestMatchProfile_DeterministicTieBreak(t *testing.T) {
	cat, _ := NewCatalog([]Profile{
		{Name: "alpha", Version: "v1", DevcontainerPath: "a", Languages: []string{"go"}, AllowedNetwork: []NetworkPolicy{NetworkRestricted}},
		{Name: "beta", Version: "v1", DevcontainerPath: "b", Languages: []string{"go"}, AllowedNetwork: []NetworkPolicy{NetworkRestricted}},
	})
	req := SandboxRequirements{Languages: []string{"go"}}
	for i := range 20 {
		p, err := MatchProfile(req, cat)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if p.Name != "alpha" {
			t.Fatalf("iter %d: tie-break drift, got %q want alpha (catalog-order)", i, p.Name)
		}
	}
}

func TestProfileAllowsPrivilege(t *testing.T) {
	p := Profile{AllowedPrivileges: []Privilege{PrivilegeDockerSocket}}
	if !ProfileAllowsPrivilege(p, PrivilegeDockerSocket) {
		t.Fatalf("expected docker-socket allowed")
	}
	if ProfileAllowsPrivilege(p, PrivilegePrivileged) {
		t.Fatalf("expected privileged denied")
	}
}

func TestProfileAllowsNetwork(t *testing.T) {
	// Default-deny per PR 4.1 finding M5: zero-value AllowedNetwork
	// allows nothing. NewCatalog rejects profiles with empty
	// AllowedNetwork at registration, so this branch only fires for
	// dynamically-constructed Profile literals.
	p := Profile{}
	if ProfileAllowsNetwork(p, NetworkRestricted) {
		t.Fatalf("zero-value allowlist should deny everything (default-deny)")
	}
	if ProfileAllowsNetwork(p, NetworkPublic) {
		t.Fatalf("zero-value should deny public")
	}
	// Explicit allowlist: only what's listed.
	p2 := Profile{AllowedNetwork: []NetworkPolicy{NetworkPublic}}
	if !ProfileAllowsNetwork(p2, NetworkPublic) {
		t.Fatalf("explicit allowlist of public should accept")
	}
	if ProfileAllowsNetwork(p2, NetworkRestricted) {
		t.Fatalf("explicit allowlist not including restricted should reject")
	}
}

func TestCatalogGet(t *testing.T) {
	cat := testCatalog(t)
	if _, ok := cat.Get("go-backend"); !ok {
		t.Fatalf("Get(go-backend) missed")
	}
	if _, ok := cat.Get("GO-BACKEND"); !ok {
		t.Fatalf("Get is not case-insensitive")
	}
	if _, ok := cat.Get("missing"); ok {
		t.Fatalf("Get returned hit on unknown profile")
	}
}

func TestProfileID(t *testing.T) {
	p := Profile{Name: "go-backend", Version: "v1"}
	if got := p.ID(); got != "go-backend@v1" {
		t.Fatalf("ID format drift: %q", got)
	}
}
