package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8090", "HTTP listen address")
	workspaceRoot := flag.String("workspace", "/workspace", "Workspace root directory")
	maxFileSize := flag.Int64("max-file-size", 1*1024*1024, "Maximum file size for write operations in bytes")
	maxReadBytes := flag.Int("max-read-bytes", 256*1024, "Maximum file content returned by read in bytes (truncate beyond)")
	maxOutputBytes := flag.Int("max-output-bytes", 100*1024, "Maximum stdout/stderr captured per exec in bytes (truncate beyond)")
	defaultExecTimeout := flag.Duration("exec-timeout", 30*time.Second, "Default exec timeout if request omits timeout_ms")
	maxExecTimeout := flag.Duration("max-exec-timeout", 5*time.Minute, "Maximum allowed exec timeout regardless of request")
	dockerModeFlag := flag.String("docker-mode", "none", `Docker access mode for chains running testcontainer flows: "none" (default; sandbox has no Docker access), "dood" (Docker-out-of-Docker via mounted /var/run/docker.sock). Env override: SANDBOX_DOCKER_MODE. DinD is a follow-on slice (R3.7.2.d′-dind, see ADR-034 §addendum #2).`)
	dockerModeStrict := flag.Bool("docker-mode-strict", false, `When true with --docker-mode=dood, fail-fast at boot if /var/run/docker.sock isn't accessible. Default false preserves the warn-and-continue behavior that lets operators debug a missing mount without re-rolling images; smoke tests + production deployments should set this true so a misconfigured mount surfaces at boot rather than at first chain run.`)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	mode, err := resolveDockerMode(*dockerModeFlag)
	if err != nil {
		logger.Error("invalid --docker-mode", "error", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*workspaceRoot, 0o755); err != nil {
		logger.Error("failed to create workspace root", "path", *workspaceRoot, "error", err)
		os.Exit(1)
	}

	if mode == dockerModeDooD {
		if _, err := os.Stat(dockerSocketPath); err != nil {
			if *dockerModeStrict {
				logger.Error("docker-mode=dood with --docker-mode-strict but host socket not accessible",
					"path", dockerSocketPath, "error", err)
				os.Exit(1)
			}
			// Warn-and-continue: the operator may have left the
			// mount off intentionally while debugging compose. The
			// first chain that tries `docker` will surface a clear
			// EACCES on its own.
			logger.Warn("docker-mode=dood but host socket not accessible at boot (continuing — set --docker-mode-strict to fail-fast)",
				"path", dockerSocketPath, "error", err)
		} else {
			logger.Info("docker-mode=dood; host docker socket present",
				"path", dockerSocketPath)
		}
	}

	srv := NewServer(ServerConfig{
		WorkspaceRoot:      *workspaceRoot,
		MaxFileSize:        *maxFileSize,
		MaxReadBytes:       *maxReadBytes,
		MaxOutputBytes:     *maxOutputBytes,
		DefaultExecTimeout: *defaultExecTimeout,
		MaxExecTimeout:     *maxExecTimeout,
		DockerMode:         mode,
		Logger:             logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// WriteTimeout must accommodate the longest exec the server will
	// honor (a cold-cache `mvn dependency:resolve` for a non-trivial
	// pom can run minutes). Capping below MaxExecTimeout would tear the
	// response mid-write and leave the agent loop guessing whether the
	// build succeeded. +10s grace covers JSON encoding + flush.
	httpWriteTimeout := *maxExecTimeout + 10*time.Second
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 14, // 16 KB — defense-in-depth.
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		logger.Info("shutting down sandbox server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	logger.Info("sandbox server starting", "addr", *addr, "workspace", *workspaceRoot, "docker_mode", mode)
	fmt.Fprintf(os.Stderr, "sandbox server listening on %s (docker_mode=%s)\n", *addr, mode)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
