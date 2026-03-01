// Package githubwebhook provides an HTTP input component that receives GitHub
// webhook events and publishes them to NATS JetStream.
package githubwebhook

import (
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// Config holds configuration for the GitHub webhook input component.
type Config struct {
	// HTTPPort is the port the webhook HTTP server listens on.
	HTTPPort int `json:"http_port" schema:"type:int,description:HTTP port for webhook receiver,default:8090,category:basic"`

	// Path is the URL path that receives GitHub webhook POST requests.
	Path string `json:"path" schema:"type:string,description:Webhook endpoint path,default:/github/webhook,category:basic"`

	// WebhookSecretEnv is the name of the environment variable that holds
	// the HMAC secret used to validate X-Hub-Signature-256 signatures.
	// When empty, signature validation is skipped (not recommended for production).
	WebhookSecretEnv string `json:"webhook_secret_env" schema:"type:string,description:Environment variable name for GitHub webhook HMAC secret,category:security"`

	// RepoAllowlist restricts processing to specific repositories in
	// "owner/repo" format. An empty slice accepts events from all repositories.
	RepoAllowlist []string `json:"repo_allowlist" schema:"type:array,description:Repositories to process (owner/repo format),category:basic"`

	// EventFilter restricts which GitHub event types are accepted.
	// Valid values: issues, pull_request, pull_request_review, issue_comment.
	// An empty slice falls back to the package defaults.
	EventFilter []string `json:"event_filter" schema:"type:array,description:GitHub event types to accept (issues;pull_request;pull_request_review;issue_comment),category:basic"`

	// Ports configures the output NATS subjects for each event category.
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration,category:basic"`
}

// defaultEventFilter is the set of GitHub events accepted when no explicit
// filter is configured.
var defaultEventFilter = []string{"issues", "pull_request", "pull_request_review"}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() Config {
	outputDefs := []component.PortDefinition{
		{
			Name:        "github.event.issue",
			Type:        "jetstream",
			StreamName:  "GITHUB",
			Subject:     "github.event.issue",
			Required:    false,
			Description: "GitHub issue webhook events",
		},
		{
			Name:        "github.event.pr",
			Type:        "jetstream",
			StreamName:  "GITHUB",
			Subject:     "github.event.pr",
			Required:    false,
			Description: "GitHub pull-request webhook events",
		},
		{
			Name:        "github.event.review",
			Type:        "jetstream",
			StreamName:  "GITHUB",
			Subject:     "github.event.review",
			Required:    false,
			Description: "GitHub pull-request review webhook events",
		},
		{
			Name:        "github.event.comment",
			Type:        "jetstream",
			StreamName:  "GITHUB",
			Subject:     "github.event.comment",
			Required:    false,
			Description: "GitHub issue comment webhook events",
		},
	}

	return Config{
		HTTPPort:    8090,
		Path:        "/github/webhook",
		EventFilter: defaultEventFilter,
		Ports: &component.PortConfig{
			Inputs:  []component.PortDefinition{},
			Outputs: outputDefs,
		},
	}
}

// githubWebhookSchema is the configuration schema derived from the Config struct.
var githubWebhookSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))
