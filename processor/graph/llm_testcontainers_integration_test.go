//go:build integration

package graph

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/c360/semstreams/processor/graph/llm"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// shimmyImage is the Docker image for the shimmy model inference backend.
	shimmyImage = "ghcr.io/c360studio/semshimmy:latest"

	// seminstructImage is the Docker image for the OpenAI-compatible proxy.
	seminstructImage = "ghcr.io/c360studio/seminstruct:latest"

	// semembedImage is the Docker image for the embedding service.
	semembedImage = "ghcr.io/c360studio/semembed:latest"

	// defaultLLMModel is the lightweight model used for CI testing (Qwen 0.5B, ~300MB).
	// Use the quantized model name as registered in shimmy.
	defaultLLMModel = "qwen2.5-0.5b-instruct-q4-k-m"

	// Timeouts for container startup
	shimmyStartupTimeout      = 180 * time.Second // Model download on first run
	seminstructStartupTimeout = 30 * time.Second
	semembedStartupTimeout    = 60 * time.Second
	cleanupTimeout            = 30 * time.Second
)

// LLMTestHelper manages LLM service testcontainers for integration testing.
// It starts shimmy (model inference) and seminstruct (OpenAI-compatible proxy)
// on a shared Docker network so they can communicate.
type LLMTestHelper struct {
	network     *testcontainers.DockerNetwork
	shimmy      testcontainers.Container
	seminstruct testcontainers.Container
	semembed    testcontainers.Container // optional, for embedding tests

	// URLs for test code to connect to
	SeminstructURL string // OpenAI-compatible API URL (e.g., "http://localhost:PORT/v1")
	SemembedURL    string // Embedding service URL (e.g., "http://localhost:PORT")
	ShimmyURL      string // Direct shimmy URL (usually not needed)

	// Model name for LLM client config
	Model string

	t *testing.T
}

// LLMTestOption configures LLM testcontainer behavior
type LLMTestOption func(*llmTestConfig)

type llmTestConfig struct {
	model          string
	enableEmbedder bool
	shimmyTimeout  time.Duration
}

// WithLLMModel sets a custom model (default: Qwen 0.5B for CI)
func WithLLMModel(model string) LLMTestOption {
	return func(cfg *llmTestConfig) {
		cfg.model = model
	}
}

// WithEmbedder also starts the semembed service for embedding tests
func WithEmbedder() LLMTestOption {
	return func(cfg *llmTestConfig) {
		cfg.enableEmbedder = true
	}
}

// WithShimmyTimeout sets custom startup timeout for shimmy (model download)
func WithShimmyTimeout(timeout time.Duration) LLMTestOption {
	return func(cfg *llmTestConfig) {
		cfg.shimmyTimeout = timeout
	}
}

// StartLLMServices starts the LLM testcontainer stack: shimmy + seminstruct.
// The services are connected via a Docker network so seminstruct can reach shimmy.
//
// Shimmy downloads the model on first run (~300MB for Qwen 0.5B), so allow 180s startup.
// Subsequent runs use cached models and start faster.
//
// Example:
//
//	llmHelper, err := StartLLMServices(ctx, t)
//	if err != nil {
//	    t.Skip("LLM services not available: " + err.Error())
//	}
//	defer llmHelper.Close(ctx)
//
//	client, _ := llmHelper.NewLLMClient()
//	// Use client for LLM tests...
func StartLLMServices(ctx context.Context, t *testing.T, opts ...LLMTestOption) (*LLMTestHelper, error) {
	t.Helper()

	cfg := &llmTestConfig{
		model:         defaultLLMModel,
		shimmyTimeout: shimmyStartupTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	helper := &LLMTestHelper{
		t:     t,
		Model: cfg.model,
	}

	// Create Docker network for container-to-container communication
	net, err := network.New(ctx,
		network.WithCheckDuplicate(),
		network.WithAttachable(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker network: %w", err)
	}
	helper.network = net

	// Start shimmy (model inference backend)
	// Container is assigned to helper immediately for cleanup on failure
	shimmyHost, err := helper.startShimmy(ctx, cfg)
	if err != nil {
		helper.Close(ctx)
		return nil, fmt.Errorf("failed to start shimmy: %w", err)
	}

	// Start seminstruct (OpenAI-compatible proxy)
	err = helper.startSeminstruct(ctx, shimmyHost)
	if err != nil {
		helper.Close(ctx)
		return nil, fmt.Errorf("failed to start seminstruct: %w", err)
	}

	// Optionally start semembed
	if cfg.enableEmbedder {
		err = helper.startSemembed(ctx)
		if err != nil {
			helper.Close(ctx)
			return nil, fmt.Errorf("failed to start semembed: %w", err)
		}
	}

	t.Logf("LLM services started successfully:")
	t.Logf("  Seminstruct: %s", helper.SeminstructURL)
	if helper.SemembedURL != "" {
		t.Logf("  Semembed: %s", helper.SemembedURL)
	}

	return helper, nil
}

// startShimmy starts the shimmy model inference container.
// The container is assigned to h.shimmy immediately for cleanup on failure.
// Returns the internal network hostname for seminstruct to connect.
func (h *LLMTestHelper) startShimmy(ctx context.Context, cfg *llmTestConfig) (string, error) {
	req := testcontainers.ContainerRequest{
		Image:        shimmyImage,
		ExposedPorts: []string{"11435/tcp"}, // shimmy listens on 11435
		Env: map[string]string{
			"RUST_LOG": "info",
			"MODEL":    cfg.model,
		},
		Networks: []string{h.network.Name},
		NetworkAliases: map[string][]string{
			h.network.Name: {"shimmy"},
		},
		// Explicit DNS servers for external resolution (Hugging Face CDN)
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.DNS = []string{"8.8.8.8", "1.1.1.1"}
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(nat.Port("11435/tcp")),
			// Wait for /v1/models instead of /health - model list only works when model is loaded
			// /health returns OK before model is ready, causing 502 errors on inference
			wait.ForHTTP("/v1/models").
				WithPort(nat.Port("11435/tcp")).
				WithStartupTimeout(cfg.shimmyTimeout),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	// Assign immediately for cleanup, even on error
	h.shimmy = container
	if err != nil {
		return "", fmt.Errorf("shimmy container failed to start: %w", err)
	}

	// Get mapped port for external access (test code)
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get shimmy host: %w", err)
	}

	port, err := container.MappedPort(ctx, nat.Port("11435/tcp"))
	if err != nil {
		return "", fmt.Errorf("failed to get shimmy port: %w", err)
	}

	h.ShimmyURL = fmt.Sprintf("http://%s:%s", host, port.Port())

	// Return internal network hostname for seminstruct to connect
	return "shimmy:11435", nil
}

// startSeminstruct starts the seminstruct OpenAI-compatible proxy container.
// The container is assigned to h.seminstruct immediately for cleanup on failure.
func (h *LLMTestHelper) startSeminstruct(ctx context.Context, shimmyHost string) error {
	req := testcontainers.ContainerRequest{
		Image:        seminstructImage,
		ExposedPorts: []string{"8083/tcp"},
		Env: map[string]string{
			"RUST_LOG":             "info",
			"SEMINSTRUCT_LLM_URL":  fmt.Sprintf("http://%s", shimmyHost),
			"SEMINSTRUCT_LLM_TYPE": "shimmy",
		},
		Networks: []string{h.network.Name},
		NetworkAliases: map[string][]string{
			h.network.Name: {"seminstruct"},
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(nat.Port("8083/tcp")),
			wait.ForHTTP("/health").
				WithPort(nat.Port("8083/tcp")).
				WithStartupTimeout(seminstructStartupTimeout),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	// Assign immediately for cleanup, even on error
	h.seminstruct = container
	if err != nil {
		return fmt.Errorf("seminstruct container failed to start: %w", err)
	}

	// Get mapped port for test code to connect
	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get seminstruct host: %w", err)
	}

	port, err := container.MappedPort(ctx, nat.Port("8083/tcp"))
	if err != nil {
		return fmt.Errorf("failed to get seminstruct port: %w", err)
	}

	h.SeminstructURL = fmt.Sprintf("http://%s:%s/v1", host, port.Port())
	return nil
}

// startSemembed starts the embedding service container.
// The container is assigned to h.semembed immediately for cleanup on failure.
func (h *LLMTestHelper) startSemembed(ctx context.Context) error {
	req := testcontainers.ContainerRequest{
		Image:        semembedImage,
		ExposedPorts: []string{"8081/tcp"},
		Env: map[string]string{
			"RUST_LOG": "info",
		},
		Networks: []string{h.network.Name},
		NetworkAliases: map[string][]string{
			h.network.Name: {"semembed"},
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(nat.Port("8081/tcp")),
			wait.ForHTTP("/health").
				WithPort(nat.Port("8081/tcp")).
				WithStartupTimeout(semembedStartupTimeout),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	// Assign immediately for cleanup, even on error
	h.semembed = container
	if err != nil {
		return fmt.Errorf("semembed container failed to start: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get semembed host: %w", err)
	}

	port, err := container.MappedPort(ctx, nat.Port("8081/tcp"))
	if err != nil {
		return fmt.Errorf("failed to get semembed port: %w", err)
	}

	h.SemembedURL = fmt.Sprintf("http://%s:%s", host, port.Port())
	return nil
}

// NewLLMClient creates an LLM client configured to use the testcontainer services.
// The client is ready to use for chat completions.
func (h *LLMTestHelper) NewLLMClient() (*llm.OpenAIClient, error) {
	if h.SeminstructURL == "" {
		return nil, fmt.Errorf("seminstruct not started")
	}

	return llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL:    h.SeminstructURL,
		Model:      h.Model,
		Timeout:    60 * time.Second,
		MaxRetries: 2,
		Logger:     slog.Default(),
	})
}

// Close terminates all containers and cleans up the Docker network.
// Uses a fresh context to ensure cleanup completes even if the parent context is cancelled.
// Call this in a defer after StartLLMServices.
func (h *LLMTestHelper) Close(_ context.Context) {
	// Create a fresh context with timeout for cleanup operations.
	// Don't rely on the parent context which might be cancelled.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	// Collect container logs if the test failed (for debugging)
	if h.t.Failed() {
		h.dumpContainerLogs(cleanupCtx)
	}

	// Terminate in reverse order of startup
	var errs []error

	if h.semembed != nil {
		if err := h.semembed.Terminate(cleanupCtx); err != nil {
			errs = append(errs, fmt.Errorf("semembed termination: %w", err))
		}
	}

	if h.seminstruct != nil {
		if err := h.seminstruct.Terminate(cleanupCtx); err != nil {
			errs = append(errs, fmt.Errorf("seminstruct termination: %w", err))
		}
	}

	if h.shimmy != nil {
		if err := h.shimmy.Terminate(cleanupCtx); err != nil {
			errs = append(errs, fmt.Errorf("shimmy termination: %w", err))
		}
	}

	if h.network != nil {
		if err := h.network.Remove(cleanupCtx); err != nil {
			errs = append(errs, fmt.Errorf("network removal: %w", err))
		}
	}

	// Log all errors together
	if len(errs) > 0 {
		h.t.Errorf("Container cleanup errors: %v", errs)
	}
}

// dumpContainerLogs collects and logs container output for debugging failed tests.
func (h *LLMTestHelper) dumpContainerLogs(ctx context.Context) {
	dumpLogs := func(container testcontainers.Container, name string) {
		if container == nil {
			return
		}
		logs, err := container.Logs(ctx)
		if err != nil {
			h.t.Logf("Failed to get %s logs: %v", name, err)
			return
		}
		defer logs.Close()

		buf := new(strings.Builder)
		if _, err := io.Copy(buf, logs); err != nil {
			h.t.Logf("Failed to read %s logs: %v", name, err)
			return
		}

		logContent := buf.String()
		if len(logContent) > 2000 {
			// Truncate to last 2000 chars for readability
			logContent = "...(truncated)...\n" + logContent[len(logContent)-2000:]
		}
		h.t.Logf("=== %s Container Logs ===\n%s", name, logContent)
	}

	dumpLogs(h.shimmy, "Shimmy")
	dumpLogs(h.seminstruct, "Seminstruct")
	dumpLogs(h.semembed, "Semembed")
}
