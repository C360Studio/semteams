package sandboxmanager

import (
	"strings"
	"testing"
)

func TestBuiltinCatalog_Loads(t *testing.T) {
	cat, err := BuiltinCatalog("/repo")
	if err != nil {
		t.Fatalf("BuiltinCatalog failed: %v", err)
	}
	names := []string{"go-backend", "svelte-ui", "full-stack-e2e"}
	for _, n := range names {
		if _, ok := cat.Get(n); !ok {
			t.Errorf("profile %q missing from catalog", n)
		}
	}
}

func TestBuiltinCatalog_PathsRelativeToRepoRoot(t *testing.T) {
	cat, _ := BuiltinCatalog("/repo")
	p, _ := cat.Get("go-backend")
	if !strings.HasPrefix(p.DevcontainerPath, "/repo") {
		t.Fatalf("path not anchored: %q", p.DevcontainerPath)
	}
	if !strings.HasSuffix(p.DevcontainerPath, ".devcontainer/go-backend/devcontainer.json") {
		t.Fatalf("path tail wrong: %q", p.DevcontainerPath)
	}
}

func TestBuiltinCatalog_FullStackAllowsDockerSocket(t *testing.T) {
	cat, _ := BuiltinCatalog("/repo")
	p, _ := cat.Get("full-stack-e2e")
	if !ProfileAllowsPrivilege(p, PrivilegeDockerSocket) {
		t.Fatalf("full-stack-e2e must allow docker-socket")
	}
	// go-backend explicitly does NOT
	gp, _ := cat.Get("go-backend")
	if ProfileAllowsPrivilege(gp, PrivilegeDockerSocket) {
		t.Fatalf("go-backend must NOT allow docker-socket")
	}
}

func TestBuiltinCatalog_MatchSelectsExpectedProfile(t *testing.T) {
	cat, _ := BuiltinCatalog("/repo")
	cases := []struct {
		name string
		req  SandboxRequirements
		want string
	}{
		{"go only", SandboxRequirements{Languages: []string{"go"}, Tools: []string{"task"}}, "go-backend"},
		{"node only", SandboxRequirements{Languages: []string{"node"}, Tools: []string{"playwright"}}, "svelte-ui"},
		{"go+node", SandboxRequirements{Languages: []string{"go", "node"}}, "full-stack-e2e"},
		{"e2e", SandboxRequirements{Languages: []string{"go"}, Tools: []string{"docker"}}, "full-stack-e2e"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := MatchProfile(tc.req, cat)
			if err != nil {
				t.Fatalf("MatchProfile: %v", err)
			}
			if p.Name != tc.want {
				t.Fatalf("matched %q want %q", p.Name, tc.want)
			}
		})
	}
}
