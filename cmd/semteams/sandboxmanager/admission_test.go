package sandboxmanager

import (
	"strings"
	"testing"
)

func goBackendProfile() Profile {
	return Profile{
		Name:             "go-backend",
		Version:          "v1",
		DevcontainerPath: ".devcontainer/go-backend/devcontainer.json",
		Languages:        []string{"go"},
		Tools:            []string{"task"},
		AllowedNetwork:   []NetworkPolicy{NetworkRestricted, NetworkNone},
		// No privilege allowlist
	}
}

func fullStackProfile() Profile {
	return Profile{
		Name:              "full-stack-e2e",
		Version:           "v1",
		DevcontainerPath:  ".devcontainer/full-stack-e2e/devcontainer.json",
		Languages:         []string{"go", "node"},
		Tools:             []string{"task", "docker"},
		AllowedNetwork:    []NetworkPolicy{NetworkRestricted, NetworkNone, NetworkPublic},
		AllowedPrivileges: []Privilege{PrivilegeDockerSocket},
	}
}

func TestAdmission_HappyPath(t *testing.T) {
	res := AdmissionCheck(
		SandboxRequirements{Languages: []string{"go"}, Network: NetworkRestricted},
		goBackendProfile(),
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionAdmitted {
		t.Fatalf("expected admitted, got %s; reasons=%v", res.Outcome, res.Reasons())
	}
}

func TestAdmission_DockerSocketOnNonAllowingProfile_Denied(t *testing.T) {
	res := AdmissionCheck(
		SandboxRequirements{Privileges: []Privilege{PrivilegeDockerSocket}},
		goBackendProfile(),
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionDenied {
		t.Fatalf("expected denied, got %s", res.Outcome)
	}
	if !contains(res.DeniedReasons, "docker-socket") {
		t.Fatalf("denied reason missing docker-socket: %v", res.DeniedReasons)
	}
}

func TestAdmission_DockerSocketOnAllowingProfile_Pending(t *testing.T) {
	res := AdmissionCheck(
		SandboxRequirements{Privileges: []Privilege{PrivilegeDockerSocket}},
		fullStackProfile(),
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionPending {
		t.Fatalf("expected pending, got %s", res.Outcome)
	}
	if !contains(res.PendingReasons, "docker-socket") {
		t.Fatalf("pending reason missing docker-socket: %v", res.PendingReasons)
	}
}

func TestAdmission_PrivilegedRequiresOperatorFlag(t *testing.T) {
	// Operator flag off → denied even if profile allows
	prof := fullStackProfile()
	prof.AllowedPrivileges = []Privilege{PrivilegeDockerSocket, PrivilegePrivileged}
	res := AdmissionCheck(
		SandboxRequirements{Privileges: []Privilege{PrivilegePrivileged}},
		prof,
		AdmissionPolicy{AllowPrivilegedContainers: false},
	)
	if res.Outcome != AdmissionDenied {
		t.Fatalf("expected denied without operator flag, got %s", res.Outcome)
	}

	// Operator flag on, profile allows → pending (still needs review)
	res = AdmissionCheck(
		SandboxRequirements{Privileges: []Privilege{PrivilegePrivileged}},
		prof,
		AdmissionPolicy{AllowPrivilegedContainers: true},
	)
	if res.Outcome != AdmissionPending {
		t.Fatalf("expected pending with operator flag, got %s; reasons=%v", res.Outcome, res.PendingReasons)
	}

	// Operator flag on, profile rejects → denied
	prof.AllowedPrivileges = []Privilege{PrivilegeDockerSocket}
	res = AdmissionCheck(
		SandboxRequirements{Privileges: []Privilege{PrivilegePrivileged}},
		prof,
		AdmissionPolicy{AllowPrivilegedContainers: true},
	)
	if res.Outcome != AdmissionDenied {
		t.Fatalf("expected denied when profile rejects, got %s", res.Outcome)
	}
}

func TestAdmission_PublicNetworkPathway(t *testing.T) {
	// Public on profile that doesn't allow → denied
	res := AdmissionCheck(
		SandboxRequirements{Network: NetworkPublic},
		goBackendProfile(),
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionDenied {
		t.Fatalf("expected denied, got %s", res.Outcome)
	}

	// Public on allowing profile, operator flag off → pending
	res = AdmissionCheck(
		SandboxRequirements{Network: NetworkPublic},
		fullStackProfile(),
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionPending {
		t.Fatalf("expected pending, got %s", res.Outcome)
	}

	// Public on allowing profile, operator flag on → admitted
	res = AdmissionCheck(
		SandboxRequirements{Network: NetworkPublic},
		fullStackProfile(),
		AdmissionPolicy{AllowPublicNetwork: true},
	)
	if res.Outcome != AdmissionAdmitted {
		t.Fatalf("expected admitted, got %s", res.Outcome)
	}
}

func TestAdmission_NetworkRestrictedDeniedWhenProfileDisallows(t *testing.T) {
	prof := Profile{
		Name:             "no-network",
		Version:          "v1",
		DevcontainerPath: "x",
		AllowedNetwork:   []NetworkPolicy{NetworkNone}, // only none
	}
	res := AdmissionCheck(
		SandboxRequirements{Network: NetworkRestricted},
		prof,
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionDenied {
		t.Fatalf("expected denied, got %s", res.Outcome)
	}
}

func TestAdmission_SecretMustBePreProvisioned(t *testing.T) {
	res := AdmissionCheck(
		SandboxRequirements{Secrets: []string{"OPENAI_API_KEY"}},
		goBackendProfile(),
		AdmissionPolicy{AvailableSecrets: []string{"GITHUB_TOKEN"}},
	)
	if res.Outcome != AdmissionDenied {
		t.Fatalf("expected denied, got %s", res.Outcome)
	}
	if !contains(res.DeniedReasons, "OPENAI_API_KEY") {
		t.Fatalf("denied reason missing secret name: %v", res.DeniedReasons)
	}
}

func TestAdmission_SecretProvisionedAdmits(t *testing.T) {
	res := AdmissionCheck(
		SandboxRequirements{Secrets: []string{"OPENAI_API_KEY"}},
		goBackendProfile(),
		AdmissionPolicy{AvailableSecrets: []string{"OPENAI_API_KEY", "GITHUB_TOKEN"}},
	)
	if res.Outcome != AdmissionAdmitted {
		t.Fatalf("expected admitted, got %s; reasons=%v", res.Outcome, res.Reasons())
	}
}

func TestAdmission_SecretMatchCaseSensitive(t *testing.T) {
	// Per PR 4.1 finding C4: requirement-side secrets are pinned to
	// [A-Z_][A-Z0-9_]* by Validate; a sloppy operator config with
	// lowercase entry must NOT admit a canonical-shaped request.
	res := AdmissionCheck(
		SandboxRequirements{Secrets: []string{"OPENAI_API_KEY"}},
		goBackendProfile(),
		AdmissionPolicy{AvailableSecrets: []string{"openai_api_key"}},
	)
	if res.Outcome == AdmissionAdmitted {
		t.Fatalf("case-sloppy policy entry must not admit canonical requirement; got %s", res.Outcome)
	}
}

func TestAdmission_PendingTakesAdmittedToPending(t *testing.T) {
	// Mix: admitted thing + pending thing → overall pending
	res := AdmissionCheck(
		SandboxRequirements{
			Languages:  []string{"go", "node"},
			Privileges: []Privilege{PrivilegeDockerSocket},
		},
		fullStackProfile(),
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionPending {
		t.Fatalf("expected pending (mixed), got %s", res.Outcome)
	}
}

func TestAdmission_DeniedBeatsPending(t *testing.T) {
	// Mix: pending thing + denied thing → overall denied
	res := AdmissionCheck(
		SandboxRequirements{
			Privileges: []Privilege{PrivilegeDockerSocket},
			Secrets:    []string{"NOT_PROVISIONED"},
		},
		fullStackProfile(),
		AdmissionPolicy{},
	)
	if res.Outcome != AdmissionDenied {
		t.Fatalf("expected denied beats pending, got %s", res.Outcome)
	}
}

func TestAdmission_ReasonsStableUnderInputReorder(t *testing.T) {
	a := AdmissionCheck(
		SandboxRequirements{
			Privileges: []Privilege{PrivilegePrivileged, PrivilegeDockerSocket},
		},
		goBackendProfile(),
		AdmissionPolicy{},
	)
	b := AdmissionCheck(
		SandboxRequirements{
			Privileges: []Privilege{PrivilegeDockerSocket, PrivilegePrivileged},
		},
		goBackendProfile(),
		AdmissionPolicy{},
	)
	if got := strings.Join(a.DeniedReasons, "|"); got != strings.Join(b.DeniedReasons, "|") {
		t.Fatalf("DeniedReasons order drift:\n  a=%v\n  b=%v", a.DeniedReasons, b.DeniedReasons)
	}
}

func contains(s []string, sub string) bool {
	for _, x := range s {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}
