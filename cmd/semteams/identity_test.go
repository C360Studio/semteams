package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
)

func TestBannerUsesSemTeamsIdentity(t *testing.T) {
	if strings.Contains(strings.ToLower(banner), "semstreams") {
		t.Fatalf("banner still contains semstreams identity:\n%s", banner)
	}
	if strings.Contains(strings.ToLower(banner), "semantic stream processing") {
		t.Fatalf("banner still contains framework tagline:\n%s", banner)
	}
	if !strings.Contains(banner, "always-on program manager for a configurable portfolio") {
		t.Fatalf("banner missing SemTeams tagline:\n%s", banner)
	}
	// SEMTEAMS has 8 letters, whereas SEMSTREAMS had 10 (included R: "|  _ <|").
	if strings.Contains(banner, "|  _ <|") {
		t.Fatalf("banner ASCII art still contains the R glyph from SEMSTREAMS:\n%s", banner)
	}
}

func TestPrintDetailedHelpUsesSemTeamsHeaderAndPreservesEnvContract(t *testing.T) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
	})

	printDetailedHelp()
	_ = w.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	help := string(out)

	if !strings.HasPrefix(help, "semteams - Always-on program manager for a configurable portfolio\n") {
		t.Fatalf("unexpected help header:\n%s", help)
	}
	if strings.Contains(help, "Semantic Stream Processing") {
		t.Fatalf("help still contains Semantic Stream Processing:\n%s", help)
	}
	if !strings.Contains(help, "SEMSTREAMS_CONFIG=/etc/semstreams/config.json") {
		t.Fatalf("help must preserve SEMSTREAMS_CONFIG=/etc/semstreams/config.json contract:\n%s", help)
	}
}

func TestSetupLoggerStampsSemTeamsServiceIdentity(t *testing.T) {
	nc, err := natsclient.NewClient("nats://127.0.0.1:4222")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = origStdout
	})

	logger := setupLogger("info", "json", nc, nil)
	logger.Info("SemTeams ready")
	_ = w.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out), &entry); err != nil {
		t.Fatalf("unmarshal log line %q: %v", string(out), err)
	}
	if got := entry["service"]; got != "semteams" {
		t.Fatalf("service = %v, want semteams", got)
	}
	if got := entry["msg"]; got != "SemTeams ready" {
		t.Fatalf("msg = %v, want SemTeams ready", got)
	}
}

func TestEnsureServiceManagerConfigUsesSemTeamsAPIIdentity(t *testing.T) {
	cfg := &config.Config{}
	ensureServiceManagerConfig(cfg)

	svcCfg, ok := cfg.Services["service-manager"]
	if !ok {
		t.Fatal("service-manager config not populated")
	}

	var decoded struct {
		ServerInfo map[string]string `json:"server_info"`
	}
	if err := json.Unmarshal(svcCfg.Config, &decoded); err != nil {
		t.Fatalf("unmarshal service-manager config: %v", err)
	}
	if got := decoded.ServerInfo["title"]; got != "SemTeams API" {
		t.Fatalf("server_info.title = %q, want %q", got, "SemTeams API")
	}
	if got := decoded.ServerInfo["description"]; got != "Always-on program manager for a configurable portfolio" {
		t.Fatalf("server_info.description = %q, want %q", got, "Always-on program manager for a configurable portfolio")
	}
}

func TestRunUntilShutdownLogsSemTeamsShutdownComplete(t *testing.T) {
	var buf bytes.Buffer
	origDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(origDefault)
	})

	p := &lifecycleProbe{signal: make(chan struct{})}
	if err := runUntilShutdown(t.Context(), p, p.signal, time.Second); err != nil {
		t.Fatal(err)
	}

	logs := buf.String()
	if !strings.Contains(logs, `"msg":"SemTeams shutdown complete"`) {
		t.Fatalf("expected SemTeams shutdown complete in logs, got:\n%s", logs)
	}
	if strings.Contains(logs, "SemStreams") {
		t.Fatalf("unexpected SemStreams string in shutdown logs:\n%s", logs)
	}
}
