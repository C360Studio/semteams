package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Split so this file does not contain the shape the guard refuses.
const (
	background = "context." + "Background()"
	todo       = "context." + "TODO()"
)

// fakeBin writes executable scripts named by tools into a fresh directory and returns an
// environment whose PATH starts with it, so a script under test runs the fakes instead of the
// real tools.
func fakeBin(t *testing.T, tools map[string]string, extra ...string) []string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range tools {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") {
			kv = "PATH=" + dir + ":" + strings.TrimPrefix(kv, "PATH=")
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

// copyScript copies scripts/<name> from the repository into scripts/ under a fresh throwaway
// root and returns that root.
func copyScript(t *testing.T, name string) string {
	t.Helper()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(repoRoot, "scripts", name))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", name), src, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeTestTree creates files under a fresh temporary root for sensitivity tests.
func writeTestTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func requireViolation(t *testing.T, violations []string, fragments ...string) {
	t.Helper()
	for _, v := range violations {
		all := true
		for _, f := range fragments {
			if !strings.Contains(v, f) {
				all = false
				break
			}
		}
		if all {
			return
		}
	}
	t.Fatalf("no violation mentions %q; got %d violation(s):\n  %s",
		fragments, len(violations), strings.Join(violations, "\n  "))
}

func requireNoViolations(t *testing.T, what string, violations []string) {
	t.Helper()
	if len(violations) > 0 {
		t.Fatalf("%s violations:\n  %s", what, strings.Join(violations, "\n  "))
	}
}

// The bounded-cleanup guard is a script so `task lint` / `task validate` runs it; this test proves
// it fires on the #1417 shape and stays quiet on the bounded form, in a throwaway repository.
func TestCleanupRootsCheckSensitivity(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	script := filepath.Join(repoRoot, "scripts", "cleanup-roots-check.sh")
	run := func(t *testing.T, files map[string]string) (string, error) {
		t.Helper()
		root := writeTestTree(t, files)
		if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		out, err := exec.Command("bash", script, root).CombinedOutput()
		return string(out), err
	}

	bounded := "package x\n\nfunc TestX(t *testing.T) {\n" +
		"\tctx, cancel := context.WithTimeout(context.Background(), time.Second)\n\tdefer cancel()\n\tdefer o.Stop(ctx)\n}\n"
	if out, err := run(t, map[string]string{"x/x_test.go": bounded,
		"x/x.go": "package x\n\nfunc f() { o.Stop(" + background + ") }\n"}); err != nil {
		t.Fatalf("bounded cleanup and non-test code were refused: %v\n%s", err, out)
	}

	for _, seeded := range []string{
		"\tdefer o.Stop(" + background + ")",
		"\tt.Cleanup(func() { _ = conn.Close(" + todo + ") })",
		"\trequire(c.Terminate(" + background + "))",
	} {
		out, err := run(t, map[string]string{"x/x_test.go": "package x\n\nfunc TestX(t *testing.T) {\n" + seeded + "\n}\n"})
		if err == nil || !strings.Contains(out, "x/x_test.go:4:") {
			t.Errorf("seeded %q: err=%v, want exit 1 naming x/x_test.go:4\n%s", seeded, err, out)
		}
	}
}
