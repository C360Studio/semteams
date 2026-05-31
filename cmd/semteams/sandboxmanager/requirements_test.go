package sandboxmanager

import (
	"strings"
	"testing"
)

func TestRequirementsValidate_HappyPath(t *testing.T) {
	r := SandboxRequirements{
		Languages: []string{"go", "node"},
		Tools:     []string{"task", "gh"},
		Services:  []string{"nats"},
		Network:   NetworkRestricted,
		Secrets:   []string{"OPENAI_API_KEY"},
		Mounts:    []MountClass{MountWorkspaceWrite},
		Verification: []VerifyProbe{
			{Name: "go", Command: "go version", ExpectExit: 0},
			{Name: "task", Command: "task --list", ExpectExit: 0},
		},
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate failed on happy path: %v", err)
	}
}

func TestRequirementsValidate_RejectsBadTokens(t *testing.T) {
	cases := []struct {
		name string
		req  SandboxRequirements
		want string
	}{
		{
			name: "language whitespace",
			req:  SandboxRequirements{Languages: []string{"go", " "}},
			want: "languages[1]",
		},
		{
			name: "tool with space",
			req:  SandboxRequirements{Tools: []string{"task --list"}},
			want: "tools[0]",
		},
		{
			name: "service with special char",
			req:  SandboxRequirements{Services: []string{"nats!"}},
			want: "services[0]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if err == nil {
				t.Fatalf("Validate accepted invalid input %+v", tc.req)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRequirementsValidate_NetworkEnum(t *testing.T) {
	for _, ok := range []NetworkPolicy{"", NetworkRestricted, NetworkPublic, NetworkNone} {
		r := SandboxRequirements{Network: ok}
		if err := r.Validate(); err != nil {
			t.Fatalf("Network=%q rejected: %v", ok, err)
		}
	}
	r := SandboxRequirements{Network: "wide-open"}
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("expected network enum error, got %v", err)
	}
}

func TestRequirementsValidate_SecretShape(t *testing.T) {
	bad := []string{"openai-api-key", "lower_case", "WITH SPACE", ""}
	for _, s := range bad {
		r := SandboxRequirements{Secrets: []string{s}}
		if err := r.Validate(); err == nil {
			t.Fatalf("secret %q accepted", s)
		}
	}
	good := []string{"OPENAI_API_KEY", "GITHUB_TOKEN", "_PRIVATE"}
	for _, s := range good {
		r := SandboxRequirements{Secrets: []string{s}}
		if err := r.Validate(); err != nil {
			t.Fatalf("secret %q rejected: %v", s, err)
		}
	}
}

func TestRequirementsValidate_MountEnum(t *testing.T) {
	r := SandboxRequirements{Mounts: []MountClass{"host-path:/etc"}}
	if err := r.Validate(); err == nil {
		t.Fatalf("host-path mount class accepted; admission should be the only path for that")
	}
}

func TestRequirementsValidate_PrivilegeEnum(t *testing.T) {
	r := SandboxRequirements{Privileges: []Privilege{"root"}}
	if err := r.Validate(); err == nil {
		t.Fatalf("undefined privilege accepted")
	}
	ok := SandboxRequirements{Privileges: []Privilege{PrivilegeDockerSocket, PrivilegePrivileged}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("known privileges rejected: %v", err)
	}
}

func TestRequirementsValidate_ProbeShape(t *testing.T) {
	cases := []struct {
		name string
		req  SandboxRequirements
	}{
		{
			name: "empty probe name",
			req:  SandboxRequirements{Verification: []VerifyProbe{{Name: "", Command: "go version"}}},
		},
		{
			name: "probe name with space",
			req:  SandboxRequirements{Verification: []VerifyProbe{{Name: "go version", Command: "go version"}}},
		},
		{
			name: "duplicate probe names",
			req: SandboxRequirements{Verification: []VerifyProbe{
				{Name: "go", Command: "go version"},
				{Name: "GO", Command: "go env"},
			}},
		},
		{
			name: "empty probe command",
			req:  SandboxRequirements{Verification: []VerifyProbe{{Name: "go", Command: "   "}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.req.Validate(); err == nil {
				t.Fatalf("invalid probe accepted")
			}
		})
	}
}

func TestRequirementsNormalize_Idempotent(t *testing.T) {
	r := SandboxRequirements{
		Languages: []string{"GO", " node ", "go", "rust"},
		Tools:     []string{"Task", "task"},
		Network:   "",
		Mounts:    []MountClass{MountWorkspaceWrite, MountWorkspaceWrite},
		Verification: []VerifyProbe{
			{Name: "Task", Command: " task --list "},
			{Name: "go", Command: "go version"},
		},
	}
	n1 := r.Normalize()
	n2 := n1.Normalize()

	if r.Hash() != n1.Hash() {
		t.Fatalf("hash drift under normalize: %s vs %s", r.Hash(), n1.Hash())
	}
	if n1.Hash() != n2.Hash() {
		t.Fatalf("Normalize not idempotent: %s vs %s", n1.Hash(), n2.Hash())
	}
	if n1.Network != NetworkRestricted {
		t.Fatalf("zero-value Network not defaulted to restricted: got %q", n1.Network)
	}
	if got := strings.Join(n1.Languages, ","); got != "go,node,rust" {
		t.Fatalf("Languages not deduped+sorted: %q", got)
	}
	if len(n1.Mounts) != 1 {
		t.Fatalf("Mounts not deduped: %v", n1.Mounts)
	}
	if got := n1.Verification[0].Name; got != "go" {
		t.Fatalf("probes not sorted by name: %v", n1.Verification)
	}
	if got := n1.Verification[1].Command; got != "task --list" {
		t.Fatalf("probe command not trimmed: %q", got)
	}
}

func TestRequirementsHash_StableUnderReordering(t *testing.T) {
	a := SandboxRequirements{
		Languages: []string{"go", "node"},
		Tools:     []string{"task", "gh"},
		Network:   NetworkRestricted,
		Verification: []VerifyProbe{
			{Name: "go", Command: "go version"},
			{Name: "task", Command: "task --list"},
		},
	}
	b := SandboxRequirements{
		Tools:     []string{"gh", "task"},
		Languages: []string{"node", "GO"},
		Network:   "", // zero defaults to restricted
		Verification: []VerifyProbe{
			{Name: "task", Command: "task --list"},
			{Name: "GO", Command: "go version"},
		},
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("equivalent requirements hashed differently:\n  a=%s\n  b=%s", a.Hash(), b.Hash())
	}
}

func TestRequirementsHash_DistinctOnRealChange(t *testing.T) {
	base := SandboxRequirements{Languages: []string{"go"}}
	extra := SandboxRequirements{Languages: []string{"go", "node"}}
	if base.Hash() == extra.Hash() {
		t.Fatalf("materially different requirements collided on hash")
	}
}

func TestRequirementsHasPrivilege(t *testing.T) {
	r := SandboxRequirements{Privileges: []Privilege{PrivilegeDockerSocket}}
	if !r.HasPrivilege(PrivilegeDockerSocket) {
		t.Fatalf("HasPrivilege(docker-socket) returned false on populated req")
	}
	if r.HasPrivilege(PrivilegePrivileged) {
		t.Fatalf("HasPrivilege(privileged) returned true on docker-socket-only req")
	}
}
