package sandboxfleet

import (
	"strings"
	"testing"
)

// TestCanonicalize_VariantConvergence pins the H2 review fix: two
// prose-framing variants of the same target must canonicalize to
// the same TargetSignature. If any normalizer drifts (whitespace,
// repo URL form, version padding, base-image case), this test
// fails loudly.
func TestCanonicalize_VariantConvergence(t *testing.T) {
	canonical, err := Canonicalize(CanonicalizeInput{
		Command:   "task test:integration",
		RepoURL:   "https://github.com/c360studio/semteams",
		RepoRef:   "main",
		Toolchain: map[string]string{"go": "1.26.0"},
		BaseImage: "ubuntu:24.04",
	})
	if err != nil {
		t.Fatalf("canonical baseline: %v", err)
	}
	canonicalHash := canonical.Hash()

	variants := []struct {
		name string
		in   CanonicalizeInput
	}{
		{
			name: "uppercase command + trailing whitespace",
			in: CanonicalizeInput{
				Command:   "  Task Test:Integration  ",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "ssh repo url",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "git@github.com:c360studio/semteams.git",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "mixed-case host + .git + trailing slash",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://GitHub.com/c360studio/semteams.git/",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "owner case mismatch",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/C360Studio/SemTeams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "toolchain version short form",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "toolchain version v-prefix",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "v1.26"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "toolchain key uppercase",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"Go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "trailing punctuation on command",
			in: CanonicalizeInput{
				Command:   "task test:integration;",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			got, err := Canonicalize(v.in)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if got.Hash() != canonicalHash {
				t.Errorf("variant %q hash drift:\n  want sig=%+v hash=%s\n  got  sig=%+v hash=%s",
					v.name, canonical, canonicalHash, got, got.Hash())
			}
		})
	}
}

// TestCanonicalize_MaterialDifferenceDivergence pins the inverse
// invariant: materially different targets must produce distinct
// hashes. Silent collisions corrupt cross-run measurements.
func TestCanonicalize_MaterialDifferenceDivergence(t *testing.T) {
	base, err := Canonicalize(CanonicalizeInput{
		Command:   "task test:integration",
		RepoURL:   "https://github.com/c360studio/semteams",
		RepoRef:   "main",
		Toolchain: map[string]string{"go": "1.26.0"},
		BaseImage: "ubuntu:24.04",
	})
	if err != nil {
		t.Fatalf("base canonicalize: %v", err)
	}

	cases := []struct {
		name string
		in   CanonicalizeInput
	}{
		{
			name: "different command",
			in: CanonicalizeInput{
				Command:   "task test:unit",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "different repo",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semspec",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "different ref",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "v1.2.3",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "different toolchain version",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.25.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "different toolchain key",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0", "node": "22.10.0"},
				BaseImage: "ubuntu:24.04",
			},
		},
		{
			name: "different base image",
			in: CanonicalizeInput{
				Command:   "task test:integration",
				RepoURL:   "https://github.com/c360studio/semteams",
				RepoRef:   "main",
				Toolchain: map[string]string{"go": "1.26.0"},
				BaseImage: "ubuntu:22.04",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Canonicalize(c.in)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if got.Hash() == base.Hash() {
				t.Errorf("expected distinct hash; got collision base=%+v variant=%+v hash=%s",
					base, got, base.Hash())
			}
		})
	}
}

// TestCanonicalize_Errors covers the rejected-input boundary.
func TestCanonicalize_Errors(t *testing.T) {
	cases := []struct {
		name    string
		in      CanonicalizeInput
		wantSub string
	}{
		{
			name:    "empty command",
			in:      CanonicalizeInput{Command: "", BaseImage: "ubuntu:24.04"},
			wantSub: "command must not be empty",
		},
		{
			name: "ref without url",
			in: CanonicalizeInput{
				Command:   "task t",
				RepoRef:   "main",
				BaseImage: "ubuntu:24.04",
			},
			wantSub: "without repo_url",
		},
		{
			name: "url without ref",
			in: CanonicalizeInput{
				Command:   "task t",
				RepoURL:   "https://github.com/o/r",
				BaseImage: "ubuntu:24.04",
			},
			wantSub: "repo_ref must be non-empty",
		},
		{
			name:    "base image :latest",
			in:      CanonicalizeInput{Command: "task t", BaseImage: "ubuntu:latest"},
			wantSub: ":latest forbidden",
		},
		{
			name:    "base image no tag",
			in:      CanonicalizeInput{Command: "task t", BaseImage: "ubuntu"},
			wantSub: "missing :tag",
		},
		{
			name: "toolchain unparseable version",
			in: CanonicalizeInput{
				Command:   "task t",
				BaseImage: "ubuntu:24.04",
				Toolchain: map[string]string{"go": "not-a-version"},
			},
			wantSub: "not a recognized semver",
		},
		{
			name:    "ssh url empty path",
			in:      CanonicalizeInput{Command: "task t", RepoURL: "git@github.com:", RepoRef: "main", BaseImage: "ubuntu:24.04"},
			wantSub: "host:path delimiter",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Canonicalize(c.in)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantSub)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), c.wantSub)
			}
		})
	}
}

func TestTargetSignature_HashStable(t *testing.T) {
	sig := TargetSignature{
		Command:   "task test:integration",
		RepoURL:   "https://github.com/c360studio/semteams",
		RepoRef:   "main",
		Toolchain: map[string]string{"go": "1.26.0", "node": "22.10.0"},
		BaseImage: "ubuntu:24.04",
	}
	a := sig.Hash()
	b := sig.Hash()
	if a != b {
		t.Errorf("hash unstable across calls: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("hash length = %d, want 64 (hex SHA-256)", len(a))
	}
}

func TestTargetSignature_HashMapKeyOrderInvariant(t *testing.T) {
	// Build two TargetSignatures with the same Toolchain map but
	// different insertion order; Hash must match (encoding/json
	// sorts map keys deterministically).
	a := TargetSignature{
		Command:   "task t",
		Toolchain: map[string]string{"go": "1.26.0", "node": "22.10.0"},
		BaseImage: "ubuntu:24.04",
	}
	b := TargetSignature{
		Command:   "task t",
		Toolchain: map[string]string{"node": "22.10.0", "go": "1.26.0"},
		BaseImage: "ubuntu:24.04",
	}
	if a.Hash() != b.Hash() {
		t.Errorf("map-key insertion order affected hash: %s vs %s", a.Hash(), b.Hash())
	}
}

func TestTargetSignature_ContainerName(t *testing.T) {
	sig, err := Canonicalize(CanonicalizeInput{
		Command:   "task test:integration",
		BaseImage: "ubuntu:24.04",
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	name := sig.ContainerName()
	if !strings.HasPrefix(name, "semteams-tenant-") {
		t.Errorf("container name %q does not have semteams-tenant- prefix", name)
	}
	if len(name) != len("semteams-tenant-")+12 {
		t.Errorf("container name %q has length %d, want %d",
			name, len(name), len("semteams-tenant-")+12)
	}
}

func TestTargetSignature_EntityID(t *testing.T) {
	sig, err := Canonicalize(CanonicalizeInput{
		Command:   "task test:integration",
		BaseImage: "ubuntu:24.04",
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	id := sig.EntityID("c360", "ops")
	parts := strings.Split(id, ".")
	if len(parts) != 6 {
		t.Fatalf("entity id %q has %d parts, want 6", id, len(parts))
	}
	wantPrefix := []string{"c360", "ops", "sandbox", "tenant", "execution"}
	for i, p := range wantPrefix {
		if parts[i] != p {
			t.Errorf("entity id part %d = %q, want %q (full=%q)", i, parts[i], p, id)
		}
	}
	if parts[5] != sig.Hash() {
		t.Errorf("entity id instance segment = %q, want hash %q", parts[5], sig.Hash())
	}
}

func TestTargetSignature_TryEntityID_Invalid(t *testing.T) {
	sig := TargetSignature{Command: "task t", BaseImage: "ubuntu:24.04"}
	if _, err := sig.TryEntityID("", "ops"); err == nil {
		t.Errorf("empty org should error")
	}
	if _, err := sig.TryEntityID("c360.bad", "ops"); err == nil {
		t.Errorf("dotted org should error")
	}
	if _, err := sig.TryEntityID("c360", ""); err == nil {
		t.Errorf("empty platform should error")
	}
}

// TestCanonicalizeRepoURL_Forms isolates URL parsing to catch
// regressions in the repo canonicalizer independent of the full
// CanonicalizeInput path.
func TestCanonicalizeRepoURL_Forms(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://github.com/c360studio/semteams", "https://github.com/c360studio/semteams"},
		{"https://github.com/c360studio/semteams.git", "https://github.com/c360studio/semteams"},
		{"https://github.com/c360studio/semteams/", "https://github.com/c360studio/semteams"},
		{"https://GitHub.com/C360Studio/SemTeams", "https://github.com/c360studio/semteams"},
		{"git@github.com:c360studio/semteams.git", "https://github.com/c360studio/semteams"},
		{"git@github.com:C360Studio/SemTeams", "https://github.com/c360studio/semteams"},
		{"https://gitlab.example.com/team/proj", "https://gitlab.example.com/team/proj"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := parseRepoURL(c.in)
			if err != nil {
				t.Fatalf("parse %q: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("parse %q = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeVersion_Forms(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"1.26", "1.26.0"},
		{"v1.26", "1.26.0"},
		{"1.26.0", "1.26.0"},
		{"v1.26.0", "1.26.0"},
		{"22", "22.0.0"},
		{"1.26.0-rc.1", "1.26.0-rc.1"},
		{"1.26-rc.1", "1.26.0-rc.1"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := normalizeVersion(c.in)
			if err != nil {
				t.Fatalf("normalize %q: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("normalize %q = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCanonicalize_NoRepoLegal(t *testing.T) {
	// Shell-script profiling target: no repo, no toolchain, just
	// command + base image. Should canonicalize cleanly.
	sig, err := Canonicalize(CanonicalizeInput{
		Command:   "hyperfine 'bash myscript.sh'",
		BaseImage: "ubuntu:24.04",
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if sig.RepoURL != "" || sig.RepoRef != "" {
		t.Errorf("expected empty repo fields, got url=%q ref=%q", sig.RepoURL, sig.RepoRef)
	}
	if sig.Toolchain != nil {
		t.Errorf("expected nil Toolchain, got %v", sig.Toolchain)
	}
	if sig.Hash() == "" {
		t.Errorf("hash empty")
	}
}
