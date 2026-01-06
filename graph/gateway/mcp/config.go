package mcp

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// Config holds the MCP gateway configuration.
type Config struct {
	// Enabled determines whether the MCP gateway is active
	Enabled bool `json:"enabled" default:"false" description:"Enable MCP gateway"`

	// BindAddress is the address to bind the MCP server to (default: ":8081")
	BindAddress string `json:"bind_address" default:":8081" description:"Address to bind MCP server"`

	// TimeoutStr is the query timeout as a duration string (default: "30s")
	TimeoutStr string `json:"timeout" default:"30s" description:"Query timeout duration"`

	// Path is the URL path for MCP SSE endpoint (default: "/mcp")
	Path string `json:"path" default:"/mcp" description:"URL path for MCP endpoint"`

	// ServerName is the MCP server name exposed to clients (default: "semstreams")
	ServerName string `json:"server_name" default:"semstreams" description:"MCP server name"`

	// ServerVersion is the MCP server version (default: "1.0.0")
	ServerVersion string `json:"server_version" default:"1.0.0" description:"MCP server version"`

	// MaxRequestSize is the maximum allowed request size in bytes (default: 1MB)
	MaxRequestSize int64 `json:"max_request_size" default:"1048576" description:"Maximum request size in bytes"`

	// parsed timeout cached after Validate()
	timeout time.Duration
}

// Validate validates the configuration and sets defaults.
func (c *Config) Validate() error {
	// Set defaults
	if c.BindAddress == "" {
		c.BindAddress = ":8081"
	}

	if c.TimeoutStr == "" {
		c.TimeoutStr = "30s"
	}

	if c.Path == "" {
		c.Path = "/mcp"
	}

	if c.ServerName == "" {
		c.ServerName = "semstreams"
	}

	if c.ServerVersion == "" {
		c.ServerVersion = "1.0.0"
	}

	if c.MaxRequestSize == 0 {
		c.MaxRequestSize = 1 << 20 // 1MB
	}

	// Parse timeout
	timeout, err := time.ParseDuration(c.TimeoutStr)
	if err != nil {
		return fmt.Errorf("invalid timeout duration %q: %w", c.TimeoutStr, err)
	}

	if timeout < time.Second {
		return fmt.Errorf("timeout must be at least 1s, got %v", timeout)
	}

	if timeout > 5*time.Minute {
		return fmt.Errorf("timeout must not exceed 5m, got %v", timeout)
	}

	c.timeout = timeout

	// Validate bind address format using net.SplitHostPort
	var portStr string
	_, portStr, err = net.SplitHostPort(c.BindAddress)
	if err != nil {
		// Handle ":port" format where host is empty
		if len(c.BindAddress) > 1 && c.BindAddress[0] == ':' {
			portStr = c.BindAddress[1:]
		} else {
			return fmt.Errorf("invalid bind address %q: %w", c.BindAddress, err)
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be 1-65535, got %q", portStr)
	}

	// Validate path starts with /
	if c.Path[0] != '/' {
		return fmt.Errorf("path must start with /, got %q", c.Path)
	}

	// Validate max request size
	if c.MaxRequestSize < 1024 {
		return fmt.Errorf("max_request_size must be at least 1KB, got %d", c.MaxRequestSize)
	}

	return nil
}

// Timeout returns the parsed timeout duration.
func (c *Config) Timeout() time.Duration {
	if c.timeout == 0 {
		// Default if Validate() wasn't called
		return 30 * time.Second
	}
	return c.timeout
}
