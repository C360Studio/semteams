// Package client provides HTTP clients for SemStreams E2E tests
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/config"
)

// ObservabilityClient interacts with SemStreams component management endpoints
type ObservabilityClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewObservabilityClient creates a new client for SemStreams observability endpoints
func NewObservabilityClient(baseURL string) *ObservabilityClient {
	return &ObservabilityClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: config.DefaultTestConfig.Timeout,
		},
	}
}

// PlatformHealth represents overall platform health status
type PlatformHealth struct {
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ComponentInfo represents a single component's information
// Matches SemStreams /components/list API response format
type ComponentInfo struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Enabled   bool   `json:"enabled"`
	State     string `json:"state"`
	Healthy   bool   `json:"healthy"`
	LastError string `json:"last_error,omitempty"`
}

// GetPlatformHealth retrieves overall platform health
func (c *ObservabilityClient) GetPlatformHealth(ctx context.Context) (*PlatformHealth, error) {
	url := c.baseURL + config.ServicePaths.Health

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Health endpoint may return 503 when unhealthy but still have valid JSON
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var health PlatformHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &health, nil
}

// GetComponents retrieves information about all managed components
func (c *ObservabilityClient) GetComponents(ctx context.Context) ([]ComponentInfo, error) {
	url := c.baseURL + config.ComponentPaths.List

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var components []ComponentInfo
	if err := json.NewDecoder(resp.Body).Decode(&components); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return components, nil
}

// WaitForComponentHealthy waits until a specific component reports healthy status.
// This is useful after Docker compose --wait passes (which only checks /health endpoint)
// but before individual components like graph processor have finished initialization.
func (c *ObservabilityClient) WaitForComponentHealthy(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	var lastState string

	for time.Now().Before(deadline) {
		components, err := c.GetComponents(ctx)
		if err != nil {
			lastErr = err
		} else {
			for _, comp := range components {
				if comp.Name == name {
					lastState = comp.State
					if comp.Healthy {
						return nil
					}
					break
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	if lastErr != nil {
		return fmt.Errorf("component %s not healthy after %v: last error: %w", name, timeout, lastErr)
	}
	return fmt.Errorf("component %s not healthy after %v: last state: %s", name, timeout, lastState)
}

// WaitForAllComponentsHealthy waits until all components report healthy status.
func (c *ObservabilityClient) WaitForAllComponentsHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	var unhealthyComponents []string

	for time.Now().Before(deadline) {
		components, err := c.GetComponents(ctx)
		if err != nil {
			lastErr = err
		} else {
			unhealthyComponents = nil
			allHealthy := true
			for _, comp := range components {
				if !comp.Healthy {
					allHealthy = false
					unhealthyComponents = append(unhealthyComponents, fmt.Sprintf("%s(%s)", comp.Name, comp.State))
				}
			}
			if allHealthy {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	if lastErr != nil {
		return fmt.Errorf("components not healthy after %v: last error: %w", timeout, lastErr)
	}
	return fmt.Errorf("components not healthy after %v: unhealthy: %v", timeout, unhealthyComponents)
}

// CountFileOutputLines counts lines in file output inside a container using docker exec.
// The containerName should match the container running the file output component.
// The pattern is the file glob pattern (e.g., "/tmp/streamkit-test*.jsonl").
// Returns 0 if files don't exist (not an error - just means no output yet).
func (c *ObservabilityClient) CountFileOutputLines(
	ctx context.Context,
	containerName string,
	pattern string,
) (int, error) {
	// Use docker exec to count lines in the file(s)
	// Shell is needed for glob expansion
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"sh", "-c", fmt.Sprintf("cat %s 2>/dev/null | wc -l", pattern))

	output, err := cmd.Output()
	if err != nil {
		// If the command fails (e.g., no files match), return 0
		// This is not an error - just means no output files yet
		return 0, nil
	}

	// Parse the line count from output
	countStr := strings.TrimSpace(string(output))
	if countStr == "" {
		return 0, nil
	}

	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, fmt.Errorf("parsing line count %q: %w", countStr, err)
	}

	return count, nil
}
