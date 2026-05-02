package main

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"time"
)

// execCommand runs command via `bash -c` in workDir with a timeout.
// stdout/stderr are captured up to maxOutput bytes (truncate beyond).
//
// On timeout, the entire process group is SIGKILL'd so child processes
// (e.g., maven spawning javac) don't outlive the request. SysProcAttr
// Setpgid makes that one-syscall cleanup possible.
//
// Returns exit code 0 on success, the command's exit code on failure,
// or -1 on timeout / non-exit-error failures.
//
// Environment is intentionally narrow: PATH, HOME, GOPATH, GOMODCACHE,
// NODE_PATH, JAVA_HOME, MAVEN_OPTS. The container's user shell is
// otherwise unconfigured; this gives Maven, Go, npm, and javac the
// minimum env they each expect.
func execCommand(ctx context.Context, workDir, command string, timeout time.Duration, maxOutput int) execResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workDir
	cmd.Env = []string{
		"PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/go/bin",
		"HOME=/home/sandbox",
		"GOPATH=/go",
		"GOMODCACHE=/go/pkg/mod",
		"NODE_PATH=/usr/local/lib/node_modules",
		// Arch-agnostic symlink set up in docker/sandbox.Dockerfile;
		// resolves to /usr/lib/jvm/java-21-openjdk-{amd64,arm64}.
		"JAVA_HOME=/usr/lib/jvm/java-21-openjdk",
		// JVM heap cap so a runaway build can't blow past the
		// container's 4 GB cap. Tunable via mvn -Xmx if a chain needs more.
		"MAVEN_OPTS=-Xmx2g",
		// Disable the Gradle daemon. Multiple chains share ~/.gradle
		// (cache volume, ADR-032 §18) and the daemon's exclusive
		// FileLock at ~/.gradle/daemon/<ver>/registry.bin would
		// serialise or hang concurrent builds. The 3–5s JVM-startup
		// penalty per `gradle` invocation is acceptable for the demo.
		"GRADLE_OPTS=-Dorg.gradle.daemon=false",
		// Force UTF-8 locale so JDK 21's default `C` locale doesn't
		// mangle non-ASCII filenames during Maven / Gradle copy tasks.
		// Same surface that TestResolveChainPath/unicode-name guards
		// at the file API.
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		// Pin TMPDIR so toolchains that read it (some npm postinstall
		// scripts, Gradle worker forks) land on the tmpfs at /tmp
		// rather than wherever bash defaults.
		"TMPDIR=/tmp",
	}

	// Setpgid lets us SIGKILL the whole tree on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout := &cappedWriter{max: maxOutput}
	stderr := &cappedWriter{max: maxOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	timedOut := ctx.Err() == context.DeadlineExceeded

	exitCode := 0
	switch {
	case timedOut:
		if cmd.Process != nil {
			// Negative PID targets the process group set up by Setpgid.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		exitCode = -1
	case err != nil:
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return execResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		TimedOut: timedOut,
	}
}

// cappedWriter buffers up to max bytes; writes past the cap are
// silently discarded so the caller still sees a successful Write but
// doesn't blow memory on runaway output.
type cappedWriter struct {
	buf bytes.Buffer
	max int
}

// Write returns len(p) on full discard rather than (0, nil) so
// os/exec.Cmd doesn't treat the cap as a write error and abort the
// command. Truncation is silent by design — callers see a complete-
// looking response with bounded memory cost.
func (w *cappedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}

func (w *cappedWriter) String() string {
	return w.buf.String()
}
