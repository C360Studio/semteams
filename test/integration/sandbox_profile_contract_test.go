//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
)

// TestSandboxProfileContract is the ADR-043 §Risks #6 ("Profile
// drift") gate. Each canonical profile is brought up via
// @devcontainers/cli + probed for its advertised capabilities + torn
// down. Broken profile fails CI loudly.
//
// Gated by SANDBOX_PROFILE_SMOKE=1 + a working `devcontainer` binary
// on PATH + a working Docker daemon. CI must opt-in; local devs run
// it on demand with:
//
//	SANDBOX_PROFILE_SMOKE=1 go test -tags=integration \
//	  -run TestSandboxProfileContract -v ./test/integration/...
//
// Per-profile sub-tests so failures pinpoint the broken devcontainer.
func TestSandboxProfileContract(t *testing.T) {
	if os.Getenv("SANDBOX_PROFILE_SMOKE") != "1" {
		t.Skip("set SANDBOX_PROFILE_SMOKE=1 to run sandbox profile contract test")
	}
	if _, err := exec.LookPath("devcontainer"); err != nil {
		t.Skip("devcontainer CLI not on PATH; install @devcontainers/cli first")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not on PATH; need Docker daemon for devcontainer up")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}

	cat, err := sandboxmanager.BuiltinCatalog(repoRoot)
	if err != nil {
		t.Fatalf("BuiltinCatalog: %v", err)
	}

	runner := sandboxmanager.NewCLIRunner()
	tenantRoot := t.TempDir()

	type probeSet struct {
		probes []sandboxmanager.VerifyProbe
		// expectedVerified is the subset of probe names that must
		// show in att.Verified. Empty means "all probes must verify."
		expectedVerified []string
	}

	cases := map[string]probeSet{
		"go-backend": {
			probes: []sandboxmanager.VerifyProbe{
				{Name: "go", Command: "go version", ExpectExit: 0},
				{Name: "task", Command: "task --version", ExpectExit: 0},
				{Name: "gh", Command: "gh --version", ExpectExit: 0},
				{Name: "jq", Command: "jq --version", ExpectExit: 0},
			},
		},
		"svelte-ui": {
			probes: []sandboxmanager.VerifyProbe{
				{Name: "node", Command: "node --version", ExpectExit: 0},
				{Name: "npm", Command: "npm --version", ExpectExit: 0},
				{Name: "playwright", Command: "ls $PLAYWRIGHT_BROWSERS_PATH/chromium-*", ExpectExit: 0},
			},
		},
		"full-stack-e2e": {
			probes: []sandboxmanager.VerifyProbe{
				{Name: "go", Command: "go version", ExpectExit: 0},
				{Name: "node", Command: "node --version", ExpectExit: 0},
				{Name: "task", Command: "task --version", ExpectExit: 0},
				{Name: "docker", Command: "docker --version", ExpectExit: 0},
				{Name: "playwright", Command: "ls $PLAYWRIGHT_BROWSERS_PATH/chromium-*", ExpectExit: 0},
			},
		},
	}

	for _, profile := range cat.Profiles() {
		profile := profile
		ps, ok := cases[profile.Name]
		if !ok {
			t.Errorf("no probe set registered for profile %q (add to TestSandboxProfileContract cases)", profile.Name)
			continue
		}
		t.Run(profile.Name, func(t *testing.T) {
			workspaceFolder := filepath.Join(tenantRoot, profile.Name, "workspace")
			if err := os.MkdirAll(workspaceFolder, 0o755); err != nil {
				t.Fatalf("mkdir workspace: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			ref, err := runner.Up(ctx, workspaceFolder, profile.DevcontainerPath, nil)
			if err != nil {
				t.Fatalf("devcontainer up: %v", err)
			}
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				teardown(cleanupCtx, ref.ContainerID)
			})

			for _, probe := range ps.probes {
				probe := probe
				t.Run(probe.Name, func(t *testing.T) {
					probeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
					defer cancel()
					res, err := runner.Exec(probeCtx, ref, probe.Command)
					if err != nil {
						t.Fatalf("exec: %v", err)
					}
					if res.ExitCode != probe.ExpectExit {
						t.Fatalf("probe %q: exit %d (expected %d); stderr=%s",
							probe.Name, res.ExitCode, probe.ExpectExit, res.Stderr)
					}
				})
			}
		})
	}
}

// teardown shells `docker rm -f` to clean up the spawned container.
// Probe failures shouldn't leave half-up containers eating resources.
func teardown(ctx context.Context, containerID string) {
	if containerID == "" {
		return
	}
	exec.CommandContext(ctx, "docker", "rm", "-f", containerID).Run()
}

// findRepoRoot walks up from the test binary's CWD looking for a
// `.git` directory. Allows the contract test to find
// `.devcontainer/<profile>/devcontainer.json` regardless of where
// `go test` is invoked.
func findRepoRoot() (string, error) {
	// Use runtime.Caller's directory as the starting point so the
	// walk is anchored on this test file's location even if CWD is
	// somewhere else (e.g. when go test is invoked from a parent).
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no .git found walking up from " + filepath.Dir(thisFile))
		}
		dir = parent
	}
}
