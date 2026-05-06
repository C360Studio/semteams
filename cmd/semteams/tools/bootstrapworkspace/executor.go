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
)

// ToolName is the LLM-facing tool name. Listed in the
// dev-via-spec-builder role's allowed_tools.
const ToolName = "bootstrap_workspace"

// SpecFilename is the canonical workspace-relative path the tool seeds
// the rendered spec markdown to. The persona's
// 10-bash-iteration-contract.md instructs the builder to read this path
// via `bash cat SPEC.md` after the first iteration.
const SpecFilename = "SPEC.md"

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
		Description: "Bootstrap the dev-via-spec-builder workspace. Call exactly once as iteration 1: creates the sandbox worktree at this loop's task_id and seeds the rendered spec markdown as SPEC.md in the workspace root. From iteration 2 onward, use bash to read SPEC.md and iterate. spec_path is the host-filesystem path to the rendered spec — the spawn rule provides this in your prompt via $entity.triple.dev_via_spec.artifact.path.",
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

	e.maybeWriteCommitmentsSidecar(ctx, taskID, resolvedPath)

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

// CommitmentsFilename is the workspace-relative path where the commitments
// sidecar is seeded. PR #2's preprocessor reads this path when running the
// evidence gate after a builder loop terminates.
const CommitmentsFilename = ".evidence/commitments.json"

// maybeWriteCommitmentsSidecar looks for a <slug>.commitments.json file
// adjacent to the resolved spec file (written by emitspecartifact when the
// architect emits VerificationCommitments). If present, it writes the content
// into the builder's workspace at .evidence/commitments.json via sandbox.WriteFile
// (which handles intermediate directories via os.MkdirAll on the server side).
//
// Absence is silently skipped — backward-compat for chains whose specs pre-date
// the sidecar feature. Errors reading or writing are logged at Error and do NOT
// fail the bootstrap; PR #2's preprocessor treats a missing file as a
// no-commitments summary and routes qa-reviewer to needs_clarification.
func (e *Executor) maybeWriteCommitmentsSidecar(ctx context.Context, taskID, resolvedSpecPath string) {
	sidecarPath := commitmentsSidecarPath(resolvedSpecPath)
	if sidecarPath == "" {
		return
	}
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		if os.IsNotExist(err) {
			e.logger.Debug("no commitments sidecar adjacent to spec_path; skipping",
				slog.String("sidecar_path", sidecarPath))
			return
		}
		// Permission/IO errors aren't transient but are operator-actionable —
		// Warn so a misconfigured deployment surfaces without spamming Error.
		e.logger.Warn("read commitments sidecar failed; skipping",
			slog.String("sidecar_path", sidecarPath),
			slog.String("error", err.Error()))
		return
	}
	if err := e.sandbox.WriteFile(ctx, taskID, CommitmentsFilename, string(data)); err != nil {
		e.logger.Error("write commitments sidecar into workspace failed; skipping",
			slog.String("task_id", taskID),
			slog.String("workspace_path", CommitmentsFilename),
			slog.String("error", err.Error()))
	}
}

// commitmentsSidecarPath derives the expected sidecar path from a resolved
// .md spec path. Returns "" for non-.md inputs — the in-band contract is
// always <slug>.md (rule_06 injects dev_via_spec.artifact.path), so any
// other extension is a chain-shape bug and we'd rather skip than synthesise
// a sidecar path the architect never wrote.
func commitmentsSidecarPath(resolvedSpecPath string) string {
	if filepath.Ext(resolvedSpecPath) != ".md" {
		return ""
	}
	return resolvedSpecPath[:len(resolvedSpecPath)-len(".md")] + ".commitments.json"
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
