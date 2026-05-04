package evidence

import (
	"context"
	"fmt"
	"os"
)

// KindTestFileExists is the registered kind name for the universal
// path-presence checker. Universal because every approach lands a
// test file somewhere in the workspace; this is the cheapest
// possible structural assertion.
const KindTestFileExists = "test_file_exists"

// FileExistsChecker validates that a workspace-relative path
// exists as a regular file. Args:
//
//	{"path": "src/test/java/com/example/FooIT.java"}
//
// The checker rejects directories with a clear Detail (the
// architect's commitment is meant to assert a single file's
// existence; passing a directory is an args mistake the reviewer
// should see).
type FileExistsChecker struct{}

// Check implements Checker.
func (FileExistsChecker) Check(_ context.Context, args map[string]any, ec *Context) Result {
	rel, err := requireStringArg(args, "path")
	if err != nil {
		return Result{Kind: KindTestFileExists, Status: StatusError, Detail: err.Error()}
	}
	abs, err := ec.Resolve(rel)
	if err != nil {
		return Result{Kind: KindTestFileExists, Status: StatusError, Detail: err.Error()}
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{
				Kind:   KindTestFileExists,
				Status: StatusFail,
				Detail: fmt.Sprintf("path %q does not exist in workspace", rel),
			}
		}
		return Result{
			Kind:   KindTestFileExists,
			Status: StatusError,
			Detail: fmt.Sprintf("stat %q: %v", rel, err),
		}
	}
	if info.IsDir() {
		return Result{
			Kind:   KindTestFileExists,
			Status: StatusFail,
			Detail: fmt.Sprintf("path %q is a directory; test_file_exists asserts a regular file", rel),
		}
	}
	return Result{Kind: KindTestFileExists, Status: StatusPass}
}

// requireStringArg fetches a non-empty string arg, returning a
// clear error message naming the missing field. Shared across the
// concrete checkers in this package.
func requireStringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required arg %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("arg %q must be a string (got %T)", key, v)
	}
	if s == "" {
		return "", fmt.Errorf("arg %q cannot be empty", key)
	}
	return s, nil
}
