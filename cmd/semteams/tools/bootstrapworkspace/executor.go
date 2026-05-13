package bootstrapworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/processor/agentic-tools/sandbox"

	"github.com/c360studio/semteams/cmd/semteams/testharness"
)

// ToolName is the LLM-facing tool name. Listed in the
// builder role's allowed_tools.
const ToolName = "bootstrap_workspace"

// SpecFilename is the canonical workspace-relative path the tool seeds
// the rendered spec markdown to. The persona's
// 10-bash-iteration-contract.md instructs the builder to read this path
// via `bash cat SPEC.md` after the first iteration.
const SpecFilename = "SPEC.md"

// ChecksFilename is the workspace-relative path where the checks slice
// from the architect's sidecar is projected. Slice 4's evidence preprocessor
// reads this path; exporting the constant lets contract tests pin it.
const ChecksFilename = ".evidence/checks.json"

// TestHarnessManifestFilename is the workspace-relative path where the
// resolved harness manifests from the architect's sidecar are projected.
// The builder's Testcontainers code reads this path (see persona fragment
// 30-test-harness.md); exporting the constant lets contract tests pin it.
const TestHarnessManifestFilename = ".test-harness/manifest.json"

// sidecarPayload is a minimal local reader for the <slug>.checks.json
// sidecar written by emitspecartifact. Kept local (not imported from
// emitspecartifact) so the two packages stay independently testable.
// json.RawMessage for checks lets us pass the bytes through without
// depending on the verification package here.
type sidecarPayload struct {
	Checks    json.RawMessage                         `json:"checks"`
	Harnesses map[string]testharness.ResolvedManifest `json:"harnesses,omitempty"`
}

// envOutputDir mirrors emitspecartifact's env var so both tools agree on
// where the architect's rendered specs live. Duplication preferred over
// cross-package import to keep packages independently testable.
const envOutputDir = "SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR"

// defaultOutputDir is the fallback when SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR
// is unset. Mirrors emitspecartifact's default.
const defaultOutputDir = "docs/specs"

// SandboxClient is the narrow surface this executor consumes. Production
// passes upstream's *sandbox.Client; tests inject an in-memory fake.
// Keeps the public Client surface unchanged so beta bumps don't ripple
// here.
type SandboxClient interface {
	CreateWorktree(ctx context.Context, taskID string, opts sandbox.CreateWorktreeOptions) (*sandbox.WorktreeInfo, error)
	WriteFile(ctx context.Context, taskID, path, content string) error
}

// Executor implements agentic.ToolExecutor for bootstrap_workspace.
// Stateless except for the injected sandbox client and (optional)
// allowed-spec-dir override. The injected dir lets tests point at a
// tmp directory without environment-variable side effects.
type Executor struct {
	sandbox    SandboxClient
	logger     *slog.Logger
	specDir    string // resolved at construction; tests inject directly, prod resolves from env
	specDirAbs string // filepath.Abs(specDir) for traversal check
}

// NewExecutor constructs an Executor. specDir overrides the env-resolved
// directory; pass "" in production to resolve from
// SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR or fall back to the package default.
func NewExecutor(sb SandboxClient, logger *slog.Logger, specDir string) (*Executor, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if specDir == "" {
		if envDir := strings.TrimSpace(os.Getenv(envOutputDir)); envDir != "" {
			specDir = envDir
		} else {
			specDir = defaultOutputDir
		}
	}
	abs, err := filepath.Abs(specDir)
	if err != nil {
		return nil, fmt.Errorf("resolve spec dir %s: %w", specDir, err)
	}
	return &Executor{
		sandbox:    sb,
		logger:     logger,
		specDir:    specDir,
		specDirAbs: abs,
	}, nil
}

// ListTools returns the LLM-facing schema. The single required arg
// `spec_path` is the host-filesystem path to the rendered markdown
// — the rule's prompt template substitutes
// $entity.triple.dev_via_spec.artifact.path on the architect's loop
// entity into the prompt body so the LLM sees the literal path.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Bootstrap the builder workspace. Call exactly once as iteration 1: creates the sandbox worktree at this loop's task_id and seeds the rendered spec markdown as SPEC.md in the workspace root. From iteration 2 onward, use bash to read SPEC.md and iterate. spec_path is the host-filesystem path to the rendered spec — the spawn rule provides this in your prompt via $entity.triple.dev_via_spec.artifact.path.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"spec_path": map[string]any{
					"type":        "string",
					"description": "Host-filesystem path to the rendered spec markdown (e.g. \"docs/specs/2026-05-03-osh-meshtastic-driver.md\"). Must resolve under the configured spec directory; absolute paths and traversals are rejected.",
				},
			},
			"required": []string{"spec_path"},
		},
	}}
}

// Execute parses + validates spec_path, reads the markdown from the host
// filesystem, creates the sandbox worktree at this loop's task_id, and
// seeds SPEC.md. LLM-facing arg errors land on Result.Error with
// ToolErrorInvalidArgs (path missing/traversal/not-found); transport
// failures use ToolErrorNetwork.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("unknown tool: %s", call.Name),
			ErrorKind: agentic.ToolErrorNotFound,
		}, nil
	}

	if call.LoopID == "" {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     "bootstrap_workspace invoked without a loop_id; cannot resolve sandbox task_id",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	if e.sandbox == nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     "bootstrap_workspace: SANDBOX_URL not configured at boot — sandbox client unavailable",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	specPathArg, ok := call.Arguments["spec_path"].(string)
	if !ok || strings.TrimSpace(specPathArg) == "" {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     "spec_path is required and must be a non-empty string",
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	specContent, resolvedPath, err := e.readSpec(specPathArg)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	taskID := call.LoopID

	wtInfo, err := e.sandbox.CreateWorktree(ctx, taskID, sandbox.CreateWorktreeOptions{})
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("create worktree task_id=%s: %v", taskID, err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	if err := e.sandbox.WriteFile(ctx, taskID, SpecFilename, specContent); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("seed %s into worktree task_id=%s: %v", SpecFilename, taskID, err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	// Project the sidecar into workspace files. Fail-soft: a missing or
	// unreadable sidecar is logged and skipped (chain ran without checks;
	// backward-compat). WriteFile errors are also logged and skipped so
	// the builder still boots and can surface the gap via needs_clarification.
	e.projectSidecar(ctx, taskID, resolvedPath)

	resultJSON, err := json.Marshal(map[string]any{
		"task_id":             taskID,
		"workspace_path":      wtInfo.Path,
		"workspace_branch":    wtInfo.Branch,
		"workspace_status":    wtInfo.Status,
		"spec_path":           resolvedPath,
		"spec_size":           len(specContent),
		"spec_workspace_path": SpecFilename,
	})
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal tool result: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	e.logger.Info("bootstrap_workspace seeded",
		slog.String("task_id", taskID),
		slog.String("workspace_status", wtInfo.Status),
		slog.String("spec_path", resolvedPath),
		slog.Int("spec_size", len(specContent)))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(resultJSON),
		Metadata: map[string]any{
			"task_id":             taskID,
			"workspace_status":    wtInfo.Status,
			"spec_workspace_path": SpecFilename,
		},
	}, nil
}

// readSpec validates the LLM-supplied spec_path and reads the content.
// Validation:
//
//   - Empty / whitespace → reject
//   - Absolute paths outside the configured spec dir → reject
//   - .. traversal that escapes the spec dir → reject
//   - File not found → reject (chain wiring failure; the rule should
//     have substituted a path from the architect's loop entity)
//
// Returns the file content, the resolved absolute path (for logging),
// or an error suitable for ToolErrorInvalidArgs.
func (e *Executor) readSpec(specPath string) (string, string, error) {
	// Both relative and absolute paths funnel through filepath.Abs
	// after a single Clean. Relative paths resolve against cwd; absolute
	// paths normalise. The downstream prefix check against specDirAbs
	// is the actual confinement; rooting strategy doesn't matter beyond
	// that. emitspecartifact stores the rel path relative to cwd, so
	// "docs/specs/<slug>.md" lands correctly here.
	//
	// Threat model: a defective architect or a corrupted triple. We do
	// NOT defend against a hostile attacker on the host filesystem
	// (e.g. a malicious symlink planted in the spec directory). On a
	// production deployment, SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR is
	// operator-controlled and not LLM-writable; the symlink threat
	// would already have other escalation paths.
	abs, err := filepath.Abs(filepath.Clean(specPath))
	if err != nil {
		return "", "", fmt.Errorf("resolve spec_path %q: %v", specPath, err)
	}

	// Confinement: abs must be under specDirAbs (prefix match on the
	// cleaned absolute paths, with a separator boundary so
	// "/docs/specs2/x.md" doesn't slip past "/docs/specs").
	prefix := e.specDirAbs
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if abs != e.specDirAbs && !strings.HasPrefix(abs, prefix) {
		return "", "", fmt.Errorf("spec_path %q resolves outside the allowed spec directory (%s); only paths under that directory are accepted", specPath, e.specDir)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("spec file not found at %q (resolved to %s); the spawn rule may have substituted a stale or wrong artifact path", specPath, abs)
		}
		return "", "", fmt.Errorf("read spec file %s: %v", abs, err)
	}
	return string(data), abs, nil
}

// projectSidecar reads <slug>.checks.json adjacent to the resolved spec path
// and projects its contents into the workspace as:
//   - ChecksFilename  (.evidence/checks.json) — for the evidence preprocessor (Slice 4)
//   - TestHarnessManifestFilename (.test-harness/manifest.json) — for the builder's Testcontainers config
//
// Fail-soft: missing or unreadable sidecar is logged at Debug/Warn and
// skipped. WriteFile errors are logged at Error and skipped. Bootstrap
// continues in all cases so a sidecar absence never blocks the builder.
func (e *Executor) projectSidecar(ctx context.Context, taskID, resolvedSpecPath string) {
	sidecarAbsPath := strings.TrimSuffix(resolvedSpecPath, ".md") + ".checks.json"

	data, err := os.ReadFile(sidecarAbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			e.logger.Debug("bootstrap_workspace: no sidecar found; skipping checks/harness projection",
				slog.String("sidecar_path", sidecarAbsPath))
			return
		}
		e.logger.Warn("bootstrap_workspace: sidecar read error; skipping projection",
			slog.String("sidecar_path", sidecarAbsPath),
			slog.String("error", err.Error()))
		return
	}

	var sidecar sidecarPayload
	if err := json.Unmarshal(data, &sidecar); err != nil {
		e.logger.Warn("bootstrap_workspace: sidecar unmarshal failed; skipping projection",
			slog.String("sidecar_path", sidecarAbsPath),
			slog.String("error", err.Error()))
		return
	}

	// Project checks slice when it is a non-empty JSON array. A missing
	// field or an empty array (`[]`) both mean "no checks" — skip.
	checksHasItems := false
	if len(sidecar.Checks) > 0 {
		var tmp []json.RawMessage
		if err := json.Unmarshal(sidecar.Checks, &tmp); err == nil && len(tmp) > 0 {
			checksHasItems = true
		}
	}
	if checksHasItems {
		e.writeWorkspaceFile(ctx, taskID, ChecksFilename, string(sidecar.Checks))
	}

	// Project harnesses map when present.
	if len(sidecar.Harnesses) > 0 {
		harnessJSON, err := json.MarshalIndent(sidecar.Harnesses, "", "  ")
		if err != nil {
			e.logger.Error("bootstrap_workspace: marshal harnesses failed; skipping .test-harness/manifest.json",
				slog.String("error", err.Error()))
		} else {
			e.writeWorkspaceFile(ctx, taskID, TestHarnessManifestFilename, string(harnessJSON))
		}
	}
}

// writeWorkspaceFile calls sandbox.WriteFile and logs at Error on failure.
// The caller does not propagate WriteFile errors (fail-soft per design).
func (e *Executor) writeWorkspaceFile(ctx context.Context, taskID, path, content string) {
	if err := e.sandbox.WriteFile(ctx, taskID, path, content); err != nil {
		e.logger.Error("bootstrap_workspace: WriteFile failed; workspace file not written",
			slog.String("path", path),
			slog.String("error", err.Error()))
	}
}
