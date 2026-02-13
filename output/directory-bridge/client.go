package directorybridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	oasfgenerator "github.com/c360studio/semstreams/processor/oasf-generator"
)

const (
	// maxResponseBodySize limits response body reads to prevent memory exhaustion.
	maxResponseBodySize = 1 << 20 // 1MB
)

// DirectoryClient handles communication with the AGNTCY directory service.
type DirectoryClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewDirectoryClient creates a new directory client.
func NewDirectoryClient(baseURL string) *DirectoryClient {
	return &DirectoryClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RegistrationRequest represents a request to register an agent.
type RegistrationRequest struct {
	// AgentDID is the agent's decentralized identifier.
	AgentDID string `json:"agent_did"`

	// OASFRecord is the agent's OASF specification.
	OASFRecord *oasfgenerator.OASFRecord `json:"oasf_record"`

	// TTL is the registration time-to-live in seconds.
	TTL int `json:"ttl,omitempty"`

	// Metadata contains additional registration metadata.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// RegistrationResponse represents the directory's response to a registration.
type RegistrationResponse struct {
	// Success indicates if registration succeeded.
	Success bool `json:"success"`

	// RegistrationID is the unique ID for this registration.
	RegistrationID string `json:"registration_id,omitempty"`

	// ExpiresAt is when the registration expires.
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// Error contains error details if registration failed.
	Error string `json:"error,omitempty"`
}

// HeartbeatRequest represents a registration renewal request.
type HeartbeatRequest struct {
	// RegistrationID is the registration to renew.
	RegistrationID string `json:"registration_id"`

	// AgentDID is the agent's DID.
	AgentDID string `json:"agent_did"`
}

// HeartbeatResponse represents the heartbeat response.
type HeartbeatResponse struct {
	// Success indicates if the heartbeat succeeded.
	Success bool `json:"success"`

	// ExpiresAt is the new expiration time.
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// Error contains error details if heartbeat failed.
	Error string `json:"error,omitempty"`
}

// DeregistrationRequest represents a request to deregister an agent.
type DeregistrationRequest struct {
	// RegistrationID is the registration to remove.
	RegistrationID string `json:"registration_id"`

	// AgentDID is the agent's DID.
	AgentDID string `json:"agent_did"`
}

// Register registers an agent with the directory.
func (c *DirectoryClient) Register(ctx context.Context, req *RegistrationRequest) (*RegistrationResponse, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("directory URL not configured")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/agents", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result RegistrationResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &result, fmt.Errorf("registration failed: %s", result.Error)
	}

	return &result, nil
}

// Heartbeat sends a heartbeat to renew a registration.
func (c *DirectoryClient) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("directory URL not configured")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/agents/%s/heartbeat", c.baseURL, req.RegistrationID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result HeartbeatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return &result, fmt.Errorf("heartbeat failed: %s", result.Error)
	}

	return &result, nil
}

// Deregister removes an agent from the directory.
func (c *DirectoryClient) Deregister(ctx context.Context, req *DeregistrationRequest) error {
	if c.baseURL == "" {
		return fmt.Errorf("directory URL not configured")
	}

	url := fmt.Sprintf("%s/v1/agents/%s", c.baseURL, req.RegistrationID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		return fmt.Errorf("deregistration failed: %s", string(body))
	}

	return nil
}

// Discover searches the directory for agents.
func (c *DirectoryClient) Discover(ctx context.Context, query *DiscoveryQuery) (*DiscoveryResponse, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("directory URL not configured")
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/agents/discover", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result DiscoveryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// DiscoveryQuery represents a search query for agents.
type DiscoveryQuery struct {
	// Capabilities filters by required capabilities.
	Capabilities []string `json:"capabilities,omitempty"`

	// Domains filters by domains.
	Domains []string `json:"domains,omitempty"`

	// Limit limits the number of results.
	Limit int `json:"limit,omitempty"`
}

// DiscoveryResponse contains discovered agents.
type DiscoveryResponse struct {
	// Agents are the matching agents.
	Agents []DiscoveredAgent `json:"agents"`

	// Total is the total number of matches.
	Total int `json:"total"`
}

// DiscoveredAgent represents an agent found in the directory.
type DiscoveredAgent struct {
	// AgentDID is the agent's DID.
	AgentDID string `json:"agent_did"`

	// OASFRecord is the agent's OASF specification.
	OASFRecord *oasfgenerator.OASFRecord `json:"oasf_record"`

	// RegisteredAt is when the agent registered.
	RegisteredAt time.Time `json:"registered_at"`

	// ExpiresAt is when the registration expires.
	ExpiresAt time.Time `json:"expires_at"`
}
