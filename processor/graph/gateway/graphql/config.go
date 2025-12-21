// Package graphql provides GraphQL gateway functionality as an output port of the graph processor.
package graphql

import (
	"fmt"
	"time"

	"github.com/c360/semstreams/pkg/errs"
)

// Config holds configuration for the GraphQL gateway output port.
type Config struct {
	// Enabled controls whether the GraphQL gateway is started
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable GraphQL gateway,default:false,category:basic"`

	// BindAddress is the HTTP bind address (default: ":8080")
	BindAddress string `json:"bind_address" schema:"type:string,description:HTTP bind address,default::8080,category:basic"`

	// Path is the GraphQL endpoint path (default: "/graphql")
	Path string `json:"path" schema:"type:string,description:GraphQL endpoint path,default:/graphql,category:basic"`

	// EnablePlayground enables GraphQL Playground UI (default: true)
	EnablePlayground bool `json:"enable_playground" schema:"type:bool,description:Enable GraphQL Playground,default:true,category:basic"`

	// EnableCORS enables CORS headers (default: true)
	EnableCORS bool `json:"enable_cors" schema:"type:bool,description:Enable CORS,default:true,category:advanced"`

	// CORSOrigins lists allowed CORS origins (default: ["*"])
	CORSOrigins []string `json:"cors_origins,omitempty" schema:"type:array,description:Allowed CORS origins,category:advanced"`

	// TimeoutStr is the default query timeout (default: "30s")
	TimeoutStr string `json:"timeout,omitempty" schema:"type:string,description:Query timeout,default:30s,category:advanced"`

	// MaxQueryDepth limits GraphQL query nesting depth (default: 10)
	MaxQueryDepth int `json:"max_query_depth,omitempty" schema:"type:int,description:Maximum query depth,default:10,category:advanced"`

	// timeout is the parsed duration (internal use)
	timeout time.Duration
}

// Validate ensures the configuration is valid.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if disabled
	}

	// Validate bind address
	if c.BindAddress == "" {
		c.BindAddress = ":8080"
	}

	// Validate path
	if c.Path == "" {
		c.Path = "/graphql"
	}
	if len(c.Path) == 0 || c.Path[0] != '/' {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			"path must start with /")
	}

	// Validate timeout
	if c.TimeoutStr == "" {
		c.timeout = 30 * time.Second
	} else {
		timeout, err := time.ParseDuration(c.TimeoutStr)
		if err != nil {
			return errs.WrapInvalid(err, "Config", "Validate",
				fmt.Sprintf("invalid timeout format: %s", c.TimeoutStr))
		}
		if timeout < 100*time.Millisecond || timeout > 5*time.Minute {
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
				"timeout must be between 100ms and 5m")
		}
		c.timeout = timeout
	}

	// Validate max query depth
	if c.MaxQueryDepth == 0 {
		c.MaxQueryDepth = 10
	}
	if c.MaxQueryDepth < 1 || c.MaxQueryDepth > 50 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			"max_query_depth must be between 1 and 50")
	}

	// Set CORS defaults
	if c.EnableCORS && len(c.CORSOrigins) == 0 {
		c.CORSOrigins = []string{"*"}
	}

	return nil
}

// Timeout returns the parsed timeout duration.
func (c *Config) Timeout() time.Duration {
	if c.timeout == 0 {
		return 30 * time.Second
	}
	return c.timeout
}

// DefaultConfig returns default GraphQL gateway configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:          false,
		BindAddress:      ":8080",
		Path:             "/graphql",
		EnablePlayground: true,
		EnableCORS:       true,
		CORSOrigins:      []string{"*"},
		TimeoutStr:       "30s",
		MaxQueryDepth:    10,
		timeout:          30 * time.Second,
	}
}
