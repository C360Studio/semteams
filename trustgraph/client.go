package trustgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Config configures the TrustGraph client.
type Config struct {
	// Endpoint is the TrustGraph REST API base URL.
	// Default: http://localhost:8088
	Endpoint string

	// APIKey is an optional API key for authentication.
	APIKey string

	// Timeout is the default timeout for requests.
	// Default: 30 seconds
	Timeout time.Duration

	// MaxRetries is the maximum number of retry attempts for retryable errors.
	// Default: 3
	MaxRetries int

	// RetryBaseDelay is the base delay for exponential backoff.
	// Default: 1 second
	RetryBaseDelay time.Duration

	// HTTPClient is an optional custom HTTP client.
	// If nil, a default client with the configured timeout is used.
	HTTPClient *http.Client
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Endpoint:       "http://localhost:8088",
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		RetryBaseDelay: time.Second,
	}
}

// Client is an HTTP client for TrustGraph REST APIs.
type Client struct {
	endpoint       string
	apiKey         string
	httpClient     *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

// New creates a new TrustGraph client with the given configuration.
func New(cfg Config) *Client {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:8088"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBaseDelay == 0 {
		cfg.RetryBaseDelay = time.Second
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: cfg.Timeout,
		}
	}

	return &Client{
		endpoint:       cfg.Endpoint,
		apiKey:         cfg.APIKey,
		httpClient:     httpClient,
		maxRetries:     cfg.MaxRetries,
		retryBaseDelay: cfg.RetryBaseDelay,
	}
}

// waitForBackoff waits for the retry backoff delay.
func (c *Client) waitForBackoff(ctx context.Context, attempt int) error {
	delay := c.retryBaseDelay * time.Duration(1<<(attempt-1)) // Exponential backoff
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// handleRateLimitResponse handles 429 rate limit responses.
// Returns true if the request should be retried after waiting.
func (c *Client) handleRateLimitResponse(ctx context.Context, resp *http.Response, apiErr *APIError) (bool, error) {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return false, nil
	}

	seconds, parseErr := strconv.Atoi(retryAfter)
	if parseErr != nil {
		return false, nil
	}

	apiErr.RetryAfter = seconds
	waitDuration := time.Duration(seconds) * time.Second
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(waitDuration):
		return true, nil
	}
}

// doRequest performs an HTTP request with retry logic.
func (c *Client) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
	}

	url := c.endpoint + path
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := c.waitForBackoff(ctx, attempt); err != nil {
				return nil, err
			}
		}

		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		respBody, apiErr, retry, err := c.executeRequest(ctx, method, url, bodyReader)
		if err != nil {
			lastErr = err
			if retry {
				continue
			}
			return nil, err
		}
		if apiErr != nil {
			lastErr = apiErr
			if retry {
				continue
			}
			return nil, apiErr
		}

		return respBody, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("request failed after %d attempts: %w", c.maxRetries+1, lastErr)
	}
	return nil, fmt.Errorf("request failed after %d attempts", c.maxRetries+1)
}

// executeRequest performs a single HTTP request attempt.
// Returns: response body, API error (if any), should retry, error
func (c *Client) executeRequest(ctx context.Context, method, url string, body io.Reader) ([]byte, *APIError, bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, nil, false, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, true, fmt.Errorf("http request failed: %w", err) // Retry on network errors
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, nil, true, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 400 {
		return respBody, nil, false, nil
	}

	// Handle error responses
	apiErr := &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}

	if resp.StatusCode == 429 {
		shouldRetry, waitErr := c.handleRateLimitResponse(ctx, resp, apiErr)
		if waitErr != nil {
			return nil, nil, false, waitErr
		}
		return nil, apiErr, shouldRetry, nil
	}

	// Retry on 5xx errors, don't retry other 4xx
	return nil, apiErr, resp.StatusCode >= 500, nil
}

// post performs a POST request to the given path.
func (c *Client) post(ctx context.Context, path string, body any, result any) error {
	respBody, err := c.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

// Endpoint returns the configured endpoint URL.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// WithTimeout returns a new client with the specified timeout.
func (c *Client) WithTimeout(timeout time.Duration) *Client {
	newClient := *c
	newClient.httpClient = &http.Client{
		Timeout:   timeout,
		Transport: c.httpClient.Transport,
	}
	return &newClient
}
