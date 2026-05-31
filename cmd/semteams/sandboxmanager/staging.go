package sandboxmanager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// stageDevcontainerProfile materializes the operator-curated catalog
// profile JSON into the tenant's writable workspace at
// <workspaceFolder>/.devcontainer/devcontainer.json so
// `devcontainer up`'s sibling devcontainer-lock.json write lands in
// tenant-writable space. The catalog mount is :ro by ADR-043 trust
// posture; without staging, devcontainer-cli fails with EROFS.
//
// Used by CLIRunner directly (os/exec on the backend, this Go-native
// path). SandboxAPIRunner stages via a /exec mkdir+cp because it
// operates over HTTP into a separate process — same shape, different
// IO mechanism.
//
// Returns the staged path on success.
func stageDevcontainerProfile(workspaceFolder, sourceConfigPath string) (string, error) {
	stagedDir := filepath.Join(workspaceFolder, ".devcontainer")
	if err := os.MkdirAll(stagedDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", stagedDir, err)
	}
	stagedPath := filepath.Join(stagedDir, "devcontainer.json")
	if err := copyFile(sourceConfigPath, stagedPath); err != nil {
		return "", fmt.Errorf("copy %s → %s: %w", sourceConfigPath, stagedPath, err)
	}
	return stagedPath, nil
}

// copyFile copies src to dst with 0o644 perms. Truncates dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
