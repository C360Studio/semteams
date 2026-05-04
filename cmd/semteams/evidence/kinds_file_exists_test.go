package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileExistsChecker_PassWhenFilePresent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x_test.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ec, err := NewContext(root)
	if err != nil {
		t.Fatal(err)
	}
	res := FileExistsChecker{}.Check(context.Background(), map[string]any{"path": "x_test.go"}, ec)
	if res.Status != StatusPass {
		t.Errorf("got %q, want pass; detail=%q", res.Status, res.Detail)
	}
}

func TestFileExistsChecker_FailWhenAbsent(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := FileExistsChecker{}.Check(context.Background(), map[string]any{"path": "missing.go"}, ec)
	if res.Status != StatusFail {
		t.Errorf("got %q, want fail; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "missing.go") {
		t.Errorf("detail should name the missing path: %q", res.Detail)
	}
}

func TestFileExistsChecker_FailWhenDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	ec, _ := NewContext(root)
	res := FileExistsChecker{}.Check(context.Background(), map[string]any{"path": "subdir"}, ec)
	if res.Status != StatusFail {
		t.Errorf("directory: got %q, want fail", res.Status)
	}
	if !strings.Contains(res.Detail, "directory") {
		t.Errorf("detail should call out 'directory': %q", res.Detail)
	}
}

func TestFileExistsChecker_ErrorOnMissingArgs(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := FileExistsChecker{}.Check(context.Background(), map[string]any{}, ec)
	if res.Status != StatusError {
		t.Errorf("missing args: got %q, want error", res.Status)
	}
	if !strings.Contains(res.Detail, "path") {
		t.Errorf("detail should name missing 'path' arg: %q", res.Detail)
	}
}

func TestFileExistsChecker_ErrorOnPathEscape(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := FileExistsChecker{}.Check(context.Background(), map[string]any{"path": "../etc/passwd"}, ec)
	if res.Status != StatusError {
		t.Errorf("path escape: got %q, want error; detail=%q", res.Status, res.Detail)
	}
}
