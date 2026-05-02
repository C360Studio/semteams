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
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := os.MkdirAll(*workspaceRoot, 0o755); err != nil {
		logger.Error("failed to create workspace root", "path", *workspaceRoot, "error", err)
		os.Exit(1)
	}

	srv := NewServer(ServerConfig{
		WorkspaceRoot:      *workspaceRoot,
		MaxFileSize:        *maxFileSize,
		MaxReadBytes:       *maxReadBytes,
		MaxOutputBytes:     *maxOutputBytes,
		DefaultExecTimeout: *defaultExecTimeout,
		MaxExecTimeout:     *maxExecTimeout,
		Logger:             logger,
	})
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 14, // 16 KB — defense-in-depth; .b/.c don't need to revisit.
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

	logger.Info("sandbox server starting", "addr", *addr, "workspace", *workspaceRoot)
	fmt.Fprintf(os.Stderr, "sandbox server listening on %s\n", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
