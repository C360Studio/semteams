package contract

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// repository-governance / merge-gate:
// scripts/merge-check.sh is a wrapper that execs `ateam review gate`. Gate behaviour is
// ateam's, pinned in ci.yml and scripts/doctor.sh and tested in C360Studio/ateam; this smoke
// test holds only the wrapper: which binary it runs, the arguments it passes, and the exit
// code it returns. A fake ateam records its argv and exits with $FAKE_ATEAM_EXIT.

const fakeAteam = `#!/bin/sh
printf '%s\n' "$*" > "$FAKE_ATEAM_ARGV"
exit "$FAKE_ATEAM_EXIT"
`

func TestMergeCheckWrapperExecsAteamReviewGate(t *testing.T) {
	root := copyScript(t, "merge-check.sh")
	script := filepath.Join(root, "scripts", "merge-check.sh")

	for _, tc := range []struct {
		name     string
		viaEnv   bool // name the fake by $ATEAM rather than putting it on PATH as ateam
		args     []string
		exit     int
		wantArgv string
	}{
		{"ATEAM pass", true, []string{"12"}, 0, "review gate 12"},
		{"ATEAM fail", true, []string{"12"}, 1, "review gate 12"},
		{"ATEAM unavailable", true, []string{"12"}, 2, "review gate 12"},
		{"PATH pass with extra args", false, []string{"12", "--json"}, 0, "review gate 12 --json"},
		{"PATH unavailable", false, []string{"7"}, 2, "review gate 7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argvFile := filepath.Join(t.TempDir(), "argv")
			extra := []string{"FAKE_ATEAM_ARGV=" + argvFile, "FAKE_ATEAM_EXIT=" + strconv.Itoa(tc.exit)}
			var env []string
			if tc.viaEnv {
				fake := filepath.Join(t.TempDir(), "fake-ateam")
				if err := os.WriteFile(fake, []byte(fakeAteam), 0o755); err != nil {
					t.Fatal(err)
				}
				// An ateam on PATH that fails loudly proves $ATEAM wins.
				env = fakeBin(t, map[string]string{"ateam": "#!/bin/sh\necho 'PATH ateam ran' >&2\nexit 9\n"}, extra...)
				env = append(withoutEnv(env, "ATEAM"), "ATEAM="+fake)
			} else {
				env = withoutEnv(fakeBin(t, map[string]string{"ateam": fakeAteam}, extra...), "ATEAM")
			}

			cmd := exec.Command("bash", append([]string{script}, tc.args...)...)
			cmd.Dir = root
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			status := 0
			var exitErr *exec.ExitError
			switch {
			case errors.As(err, &exitErr):
				status = exitErr.ExitCode()
			case err != nil:
				t.Fatalf("run merge-check.sh: %v", err)
			}
			if status != tc.exit {
				t.Fatalf("exit %d, want %d\n%s", status, tc.exit, out)
			}
			argv, err := os.ReadFile(argvFile)
			if err != nil {
				t.Fatalf("fake ateam did not run: %v\n%s", err, out)
			}
			if got := strings.TrimSpace(string(argv)); got != tc.wantArgv {
				t.Fatalf("ateam argv = %q, want %q", got, tc.wantArgv)
			}
		})
	}
}

func withoutEnv(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}
