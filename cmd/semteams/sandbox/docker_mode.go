package main

import (
	"fmt"
	"os"
	"strings"
)

// dockerMode is the access mode the sandbox grants to chains for
// Docker-related work (Testcontainers, `docker run`). Values are
// closed; the DinD follow-on (R3.7.2.d′-dind) adds dockerModeDinD
// without breaking the existing "none|dood" enum that compose stacks
// already consume. See ADR-034 §addendum #2 for the trust-boundary
// rationale that gates dood-first / dind-later.
type dockerMode string

const (
	dockerModeNone dockerMode = "none"

	// dockerModeDooD widens the sandbox's trust boundary in the
	// canonical Docker-out-of-Docker way: chains running inside the
	// sandbox can spawn arbitrary sibling containers on the host's
	// daemon, including privileged containers and containers that
	// bind-mount the host filesystem (e.g. `docker run --privileged
	// -v /:/host`). That path yields effective host root and is
	// equivalent to running tests as root on the developer's laptop.
	// Acceptable for the single-tenant dev workflow this slice
	// targets (ADR-034 §addendum #2). Multi-tenant deployments must
	// switch to dockerModeDinD or sysbox-runc when those land.
	dockerModeDooD dockerMode = "dood"

	// dockerSocketPath is the canonical bind-mount target inside the
	// sandbox container when dockerMode == dood. Centralising it so
	// the boot probe and any future client-config code agree on the
	// single authoritative path; if a deployment ever needs to remap
	// it (rootless docker, podman.sock), do it here, not in three
	// places.
	dockerSocketPath = "/var/run/docker.sock"

	dockerModeEnvVar = "SANDBOX_DOCKER_MODE"
)

// resolveDockerMode normalises the --docker-mode flag value with the
// SANDBOX_DOCKER_MODE env var taking precedence (compose-stack
// override of the binary's default). An unset/empty env var leaves
// the flag value alone, so an accidental `docker run -e
// SANDBOX_DOCKER_MODE=` doesn't silently widen the trust boundary.
//
// "dind" returns an explicit error rather than falling through to the
// generic "invalid" path: the value is reserved for the follow-on
// slice and operators reading the flag's help text should get a
// pointed message rather than a generic complaint.
func resolveDockerMode(flagValue string) (dockerMode, error) {
	raw := flagValue
	if env := os.Getenv(dockerModeEnvVar); env != "" {
		raw = env
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch dockerMode(raw) {
	case "":
		return dockerModeNone, nil
	case dockerModeNone, dockerModeDooD:
		return dockerMode(raw), nil
	case "dind":
		return "", fmt.Errorf("docker-mode=dind is reserved for a future release; use 'none' or 'dood'")
	default:
		return "", fmt.Errorf("docker-mode=%q invalid; want one of: none, dood", raw)
	}
}
