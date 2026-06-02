package sandboxmanager

import "context"

// MockRunner is the in-memory Runner used by mock-LLM journeys
// (test/fixtures/journeys/sandbox-mvp.yaml) and unit tests that
// want a happy-path Attestation without running devcontainer or
// docker. Wired by product_tools.go when
// SEMTEAMS_SANDBOX_RUNNER=mock.
//
// Behavior:
//   - Up returns a stable ContainerRef with a deterministic image
//     digest derived from the workspace folder so attestations
//     round-trip.
//   - Exec returns ExitCode=0 with the command echoed back as
//     Stdout so probes pass and Verified[<name>] carries a
//     reproducible value.
//
// Concurrency-safe; no internal state.
type MockRunner struct{}

// Up returns a Ready ContainerRef. ImageDigest is derived from the
// workspaceFolder so two arcs targeting the same logical work get
// the same digest (proves the SignatureFor + attestation stamping
// path is stable end-to-end).
func (MockRunner) Up(_ context.Context, workspaceFolder, _ string, _ map[string]string) (ContainerRef, error) {
	return ContainerRef{
		ContainerID:           "mock-" + fnv12(workspaceFolder),
		ImageDigest:           "sha256:mock" + fnv12(workspaceFolder),
		HostWorkspaceFolder:   workspaceFolder,
		RemoteWorkspaceFolder: "/workspaces/mock-" + fnv12(workspaceFolder),
	}, nil
}

// Exec returns a deterministic Ready ProbeResult. The command is
// echoed as Stdout so probe-name normalization + Verified map
// round-trip get exercised end-to-end in mock journeys.
func (MockRunner) Exec(_ context.Context, _ ContainerRef, command string) (ProbeResult, error) {
	return ProbeResult{
		Command:  command,
		ExitCode: 0,
		Stdout:   "mock ok: " + command,
	}, nil
}
