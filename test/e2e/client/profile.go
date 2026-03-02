// Package client provides HTTP clients for SemStreams E2E tests
package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ProfileClient captures pprof profiles from a running SemStreams instance.
// Profiles are saved to disk for analysis with `go tool pprof`.
type ProfileClient struct {
	baseURL    string
	httpClient *http.Client
	outputDir  string
}

// NewProfileClient creates a new client for pprof endpoints.
// baseURL should be the pprof server address (e.g., "http://localhost:6060").
// outputDir is where profile files will be saved.
func NewProfileClient(baseURL, outputDir string) *ProfileClient {
	return &ProfileClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // CPU profiles can take a while
		},
		outputDir: outputDir,
	}
}

// IsAvailable checks if the pprof endpoint is accessible.
func (c *ProfileClient) IsAvailable(ctx context.Context) bool {
	url := c.baseURL + "/debug/pprof/"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// CaptureHeap saves a heap profile snapshot.
// Returns the path to the saved profile file.
func (c *ProfileClient) CaptureHeap(ctx context.Context, name string) (string, error) {
	return c.capture(ctx, "/debug/pprof/heap", name+"-heap.pprof")
}

// CaptureCPU captures a CPU profile for the specified duration.
// Returns the path to the saved profile file.
func (c *ProfileClient) CaptureCPU(ctx context.Context, name string, seconds int) (string, error) {
	endpoint := fmt.Sprintf("/debug/pprof/profile?seconds=%d", seconds)
	return c.capture(ctx, endpoint, name+"-cpu.pprof")
}

// CaptureGoroutine saves goroutine stack traces.
// Returns the path to the saved profile file.
func (c *ProfileClient) CaptureGoroutine(ctx context.Context, name string) (string, error) {
	return c.capture(ctx, "/debug/pprof/goroutine", name+"-goroutine.pprof")
}

// CaptureAllocs saves a memory allocation profile.
// Returns the path to the saved profile file.
func (c *ProfileClient) CaptureAllocs(ctx context.Context, name string) (string, error) {
	return c.capture(ctx, "/debug/pprof/allocs", name+"-allocs.pprof")
}

// CaptureBlock saves a goroutine blocking profile.
// Returns the path to the saved profile file.
func (c *ProfileClient) CaptureBlock(ctx context.Context, name string) (string, error) {
	return c.capture(ctx, "/debug/pprof/block", name+"-block.pprof")
}

// CaptureMutex saves a mutex contention profile.
// Returns the path to the saved profile file.
func (c *ProfileClient) CaptureMutex(ctx context.Context, name string) (string, error) {
	return c.capture(ctx, "/debug/pprof/mutex", name+"-mutex.pprof")
}

// CaptureTrace captures an execution trace for the specified duration.
// Returns the path to the saved trace file.
func (c *ProfileClient) CaptureTrace(ctx context.Context, name string, seconds int) (string, error) {
	endpoint := fmt.Sprintf("/debug/pprof/trace?seconds=%d", seconds)
	return c.capture(ctx, endpoint, name+"-trace.out")
}

// capture fetches a profile from the given endpoint and saves it to a file.
func (c *ProfileClient) capture(ctx context.Context, endpoint, filename string) (string, error) {
	url := c.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("profile request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("profile endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	// Ensure output directory exists
	if err := os.MkdirAll(c.outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}

	// Save to file
	filePath := filepath.Join(c.outputDir, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return "", fmt.Errorf("writing profile: %w", err)
	}

	// Return absolute path for easier use
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	fmt.Printf("Captured profile: %s (%d bytes)\n", absPath, n)
	return absPath, nil
}

// CaptureBlockAndMutex captures blocking and mutex contention profiles with a common prefix.
// Useful for profiling lock contention under load.
func (c *ProfileClient) CaptureBlockAndMutex(ctx context.Context, prefix string) (map[string]string, error) {
	profiles := make(map[string]string)
	var lastErr error

	if path, err := c.CaptureBlock(ctx, prefix); err != nil {
		lastErr = err
	} else {
		profiles["block"] = path
	}

	if path, err := c.CaptureMutex(ctx, prefix); err != nil {
		lastErr = err
	} else {
		profiles["mutex"] = path
	}

	if len(profiles) == 0 && lastErr != nil {
		return nil, lastErr
	}

	return profiles, nil
}

// CaptureAll captures heap, goroutine, and allocs profiles with a common prefix.
// Useful for getting a baseline or final snapshot.
func (c *ProfileClient) CaptureAll(ctx context.Context, prefix string) (map[string]string, error) {
	profiles := make(map[string]string)
	var lastErr error

	if path, err := c.CaptureHeap(ctx, prefix); err != nil {
		lastErr = err
	} else {
		profiles["heap"] = path
	}

	if path, err := c.CaptureGoroutine(ctx, prefix); err != nil {
		lastErr = err
	} else {
		profiles["goroutine"] = path
	}

	if path, err := c.CaptureAllocs(ctx, prefix); err != nil {
		lastErr = err
	} else {
		profiles["allocs"] = path
	}

	if len(profiles) == 0 && lastErr != nil {
		return nil, lastErr
	}

	return profiles, nil
}
