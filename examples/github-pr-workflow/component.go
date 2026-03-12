package githubprworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

const (
	// WorkflowSlug identifies this workflow for correlation in TaskMessage/LoopCompletedEvent.
	WorkflowSlug = "github-issue-to-pr"

	componentName    = "pr-workflow-spawner"
	componentVersion = "0.1.0"
)

// PRWorkflowComponent spawns agent tasks for the GitHub issue-to-PR pipeline.
// It handles the Go-intensive parts of the workflow: prompt building, result parsing,
// and triple writing. The simpler reactive patterns (phase transitions, budget checks)
// are handled by JSON rules in configs/rules/github-pr-workflow/.
type PRWorkflowComponent struct {
	config     Config
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	inputPorts  []component.Port
	outputPorts []component.Port

	shutdown      chan struct{}
	done          chan struct{}
	running       bool
	mu            sync.RWMutex
	lifecycleMu   sync.Mutex
	subscriptions []*natsclient.Subscription

	messagesProcessed atomic.Int64
	errors            atomic.Int64
}

// NewComponent creates a new PRWorkflowComponent from config.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal pr-workflow config: %w", err)
	}
	cfg = cfg.withDefaults()

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", componentName)

	c := &PRWorkflowComponent{
		config:     cfg,
		natsClient: deps.NATSClient,
		logger:     logger,
		platform:   deps.Platform,
		shutdown:   make(chan struct{}),
		done:       make(chan struct{}),
	}

	// Build ports from config
	for _, p := range cfg.Ports.Inputs {
		c.inputPorts = append(c.inputPorts, component.BuildPortFromDefinition(
			component.PortDefinition{Name: p.Name, Subject: p.Subject, Type: p.Type, StreamName: p.Stream},
			component.DirectionInput,
		))
	}
	for _, p := range cfg.Ports.Outputs {
		c.outputPorts = append(c.outputPorts, component.BuildPortFromDefinition(
			component.PortDefinition{Name: p.Name, Subject: p.Subject, Type: p.Type, StreamName: p.Stream},
			component.DirectionOutput,
		))
	}

	return c, nil
}

// Initialize prepares the component.
func (c *PRWorkflowComponent) Initialize() error {
	return nil
}

// Start begins consuming events and agent completions.
func (c *PRWorkflowComponent) Start(ctx context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	c.mu.RLock()
	if c.running {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	c.logger.Info("Starting PR workflow spawner")

	for _, port := range c.inputPorts {
		subject := portSubject(port)
		if subject == "" {
			continue
		}

		var handler func(context.Context, *nats.Msg)
		switch {
		case strings.HasPrefix(subject, "github.event.issue"):
			handler = c.handleIssueEvent
		case strings.HasPrefix(subject, "agentic.loop_completed"):
			handler = c.handleLoopCompleted
		default:
			c.logger.Debug("Skipping unrecognized input port", "subject", subject)
			continue
		}

		sub, err := c.natsClient.Subscribe(ctx, subject, handler)
		if err != nil {
			return fmt.Errorf("subscribe to %s: %w", subject, err)
		}
		c.subscriptions = append(c.subscriptions, sub)
		c.logger.Debug("Subscribed", "subject", subject)
	}

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	return nil
}

// Stop performs graceful shutdown.
func (c *PRWorkflowComponent) Stop(_ time.Duration) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	c.mu.RLock()
	if !c.running {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	c.logger.Info("Stopping PR workflow spawner")

	for _, sub := range c.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			c.logger.Debug("Unsubscribe error", "error", err)
		}
	}
	c.subscriptions = nil

	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	return nil
}

// handleIssueEvent processes a GitHub issue webhook event.
func (c *PRWorkflowComponent) handleIssueEvent(ctx context.Context, msg *nats.Msg) {
	var event GitHubIssueWebhookEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		c.logger.Debug("Failed to unmarshal issue event", "error", err)
		c.errors.Add(1)
		return
	}

	if event.Action != "opened" {
		return
	}

	c.messagesProcessed.Add(1)

	// Build workflow entity ID
	entityID := WorkflowEntityID(event.Repo.Owner.Login, event.Repo.Name, event.Issue.Number)

	// Write initial entity triples
	c.writeTriple(ctx, entityID, "workflow.phase", "qualifying")
	c.writeTriple(ctx, entityID, "workflow.status", "active")
	c.writeTriple(ctx, entityID, "github.issue.number", fmt.Sprintf("%d", event.Issue.Number))
	c.writeTriple(ctx, entityID, "github.issue.title", event.Issue.Title)
	c.writeTriple(ctx, entityID, "workflow.tokens.total", "0")
	c.writeTriple(ctx, entityID, "workflow.review.rejections", "0")

	// Spawn qualifier agent
	prompt := BuildQualifierPrompt(event.Repo.Owner.Login, event.Repo.Name, event.Issue.Number, event.Issue.Title, event.Issue.Body)
	taskID := fmt.Sprintf("qualifier-%s-%d", entityID, time.Now().UnixNano())

	task := &agentic.TaskMessage{
		TaskID:       taskID,
		Role:         agentic.RoleQualifier,
		Model:        c.config.Model,
		Prompt:       prompt,
		WorkflowSlug: WorkflowSlug,
		WorkflowStep: "qualify",
	}

	c.publishTask(ctx, "agent.task.qualifier", task)
}

// handleLoopCompleted processes an agentic loop completion event.
func (c *PRWorkflowComponent) handleLoopCompleted(ctx context.Context, msg *nats.Msg) {
	// Parse the BaseMessage envelope
	var base message.BaseMessage
	if err := json.Unmarshal(msg.Data, &base); err != nil {
		c.logger.Debug("Failed to unmarshal loop completed envelope", "error", err)
		c.errors.Add(1)
		return
	}

	event, ok := base.Payload().(*agentic.LoopCompletedEvent)
	if !ok {
		// Not a LoopCompletedEvent, ignore
		return
	}

	if event.WorkflowSlug != WorkflowSlug {
		return
	}

	c.messagesProcessed.Add(1)

	switch event.WorkflowStep {
	case "qualify":
		c.handleQualifierComplete(ctx, event)
	case "develop":
		c.handleDeveloperComplete(ctx, event)
	case "review":
		c.handleReviewerComplete(ctx, event)
	default:
		c.logger.Debug("Unknown workflow step", "step", event.WorkflowStep)
	}
}

// handleQualifierComplete processes the qualifier agent's result.
func (c *PRWorkflowComponent) handleQualifierComplete(ctx context.Context, event *agentic.LoopCompletedEvent) {
	verdict := ParseQualifierResult(event.Result)

	entityID := extractEntityIDFromTaskID(event.TaskID, "qualifier-")
	if entityID == "" {
		c.logger.Debug("Cannot extract entity ID from qualifier task", "task_id", event.TaskID)
		return
	}

	c.writeTriple(ctx, entityID, "workflow.qualifier.verdict", verdict.Verdict)
	c.writeTriple(ctx, entityID, "workflow.tokens.total", fmt.Sprintf("%d", event.TokensIn+event.TokensOut))
	c.writeTriple(ctx, entityID, "workflow.phase", verdict.Verdict) // phase mirrors verdict
}

// handleDeveloperComplete processes the developer agent's result.
func (c *PRWorkflowComponent) handleDeveloperComplete(ctx context.Context, event *agentic.LoopCompletedEvent) {
	output := ParseDeveloperResult(event.Result)

	entityID := extractEntityIDFromTaskID(event.TaskID, "developer-")
	if entityID == "" {
		c.logger.Debug("Cannot extract entity ID from developer task", "task_id", event.TaskID)
		return
	}

	if output.PRNumber > 0 {
		c.writeTriple(ctx, entityID, "workflow.pr.number", fmt.Sprintf("%d", output.PRNumber))
	}
	c.accumulateTokens(ctx, entityID, event.TokensIn+event.TokensOut)
	c.writeTriple(ctx, entityID, "workflow.phase", PhaseDevComplete)
}

// handleReviewerComplete processes the reviewer agent's result.
func (c *PRWorkflowComponent) handleReviewerComplete(ctx context.Context, event *agentic.LoopCompletedEvent) {
	review := ParseReviewerResult(event.Result)

	entityID := extractEntityIDFromTaskID(event.TaskID, "reviewer-")
	if entityID == "" {
		c.logger.Debug("Cannot extract entity ID from reviewer task", "task_id", event.TaskID)
		return
	}

	c.writeTriple(ctx, entityID, "workflow.review.feedback", review.Feedback)
	c.accumulateTokens(ctx, entityID, event.TokensIn+event.TokensOut)

	switch review.Verdict {
	case "approved":
		c.writeTriple(ctx, entityID, "workflow.phase", PhaseApproved)
	case "request_changes", "reject":
		c.writeTriple(ctx, entityID, "workflow.phase", PhaseChangesRequested)
		c.incrementRejections(ctx, entityID)
	default:
		c.writeTriple(ctx, entityID, "workflow.phase", PhaseChangesRequested)
		c.incrementRejections(ctx, entityID)
	}
}

// writeTriple publishes an add_triple mutation to graph-ingest.
func (c *PRWorkflowComponent) writeTriple(ctx context.Context, entityID, predicate string, object any) {
	triple := message.Triple{
		Subject:    entityID,
		Predicate:  predicate,
		Object:     object,
		Source:     componentName,
		Timestamp:  time.Now(),
		Confidence: 1.0,
	}

	data, err := json.Marshal(triple)
	if err != nil {
		c.logger.Debug("Failed to marshal triple", "error", err)
		return
	}

	if c.natsClient != nil {
		if err := c.natsClient.Publish(ctx, "graph.mutation.triple.add", data); err != nil {
			c.logger.Debug("Failed to publish triple", "predicate", predicate, "error", err)
		}
	}
}

// publishTask publishes a TaskMessage to a NATS subject.
func (c *PRWorkflowComponent) publishTask(ctx context.Context, subject string, task *agentic.TaskMessage) {
	baseMsg := message.NewBaseMessage(task.Schema(), task, componentName)
	data, err := json.Marshal(baseMsg)
	if err != nil {
		c.logger.Debug("Failed to marshal task message", "error", err)
		c.errors.Add(1)
		return
	}

	if c.natsClient != nil {
		if err := c.natsClient.Publish(ctx, subject, data); err != nil {
			c.logger.Debug("Failed to publish task", "subject", subject, "error", err)
			c.errors.Add(1)
		}
	}
}

// accumulateTokens writes the token count. In production this would do read-modify-write.
func (c *PRWorkflowComponent) accumulateTokens(ctx context.Context, entityID string, tokens int) {
	c.writeTriple(ctx, entityID, "workflow.tokens.total", fmt.Sprintf("%d", tokens))
}

// incrementRejections writes a rejection count. In production this would increment.
func (c *PRWorkflowComponent) incrementRejections(ctx context.Context, entityID string) {
	c.writeTriple(ctx, entityID, "workflow.review.rejections", "1")
}

// WorkflowEntityID builds the 6-part entity ID for a workflow execution.
func WorkflowEntityID(org, repo string, issueNumber int) string {
	return fmt.Sprintf("%s.github.repo.%s.workflow.%d", org, repo, issueNumber)
}

// extractEntityIDFromTaskID extracts the entity ID embedded in a task ID.
// Task IDs are formatted as "<prefix><entityID>-<timestamp>".
func extractEntityIDFromTaskID(taskID, prefix string) string {
	if !strings.HasPrefix(taskID, prefix) {
		return ""
	}
	rest := taskID[len(prefix):]
	// Find the last "-" which separates the entity ID from the timestamp
	lastDash := strings.LastIndex(rest, "-")
	if lastDash <= 0 {
		return ""
	}
	return rest[:lastDash]
}

// --- Prompt Builders (exported for testing) ---

// BuildQualifierPrompt builds the prompt for the qualifier agent.
func BuildQualifierPrompt(org, repo string, issueNumber int, title, body string) string {
	return fmt.Sprintf(
		"You are an expert software engineer triaging GitHub issues.\n\n"+
			"Repository: %s/%s\n"+
			"Issue #%d: %s\n\n"+
			"Body:\n%s\n\n"+
			"Evaluate this issue and respond with a JSON object containing:\n"+
			"  - verdict: one of [qualified, rejected, not_a_bug, wont_fix, needs_info]\n"+
			"  - confidence: float between 0.0 and 1.0\n"+
			"  - severity: one of [critical, high, medium, low]\n"+
			"  - reasoning: brief explanation of your verdict\n\n"+
			"Only respond with the JSON object, no other text.",
		org, repo, issueNumber, title, body,
	)
}

// BuildDeveloperPrompt builds the prompt for the developer agent.
// If reviewFeedback is non-empty, it is included as previous review feedback.
func BuildDeveloperPrompt(org, repo string, issueNumber int, issueTitle, issueBody, qualifierVerdict string, qualifierConfidence float64, severity, reviewFeedback string) string {
	var sb strings.Builder
	sb.WriteString("You are an expert software engineer.\n\n")
	sb.WriteString(fmt.Sprintf("Repository: %s/%s\n", org, repo))
	sb.WriteString(fmt.Sprintf("Issue #%d: %s\n\n", issueNumber, issueTitle))
	sb.WriteString(fmt.Sprintf("Issue body:\n%s\n\n", issueBody))
	sb.WriteString(fmt.Sprintf("Qualifier verdict: %s (confidence %.2f, severity %s)\n\n",
		qualifierVerdict, qualifierConfidence, severity))

	if reviewFeedback != "" {
		sb.WriteString("Previous review feedback that must be addressed:\n")
		sb.WriteString(reviewFeedback)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Implement a fix for this issue. Create a feature branch, commit your changes, " +
		"and open a pull request. Respond with a JSON object containing:\n" +
		"  - branch_name: the branch you created\n" +
		"  - pr_number: the pull request number\n" +
		"  - pr_url: the URL of the pull request\n" +
		"  - files_changed: list of file paths modified\n\n" +
		"Only respond with the JSON object, no other text.")

	return sb.String()
}

// BuildReviewerPrompt builds the prompt for the reviewer agent.
func BuildReviewerPrompt(org, repo string, issueNumber int, issueTitle string, prNumber int, prURL string, filesChanged []string) string {
	return fmt.Sprintf(
		"You are a senior software engineer performing an adversarial code review.\n\n"+
			"Repository: %s/%s\n"+
			"Issue #%d: %s\n"+
			"Pull Request #%d: %s\n"+
			"Files changed: %s\n\n"+
			"Review the pull request thoroughly. Respond with a JSON object containing:\n"+
			"  - verdict: one of [approved, request_changes]\n"+
			"  - feedback: detailed review comments (required if verdict is request_changes)\n\n"+
			"Only respond with the JSON object, no other text.",
		org, repo,
		issueNumber, issueTitle,
		prNumber, prURL,
		strings.Join(filesChanged, ", "),
	)
}

// --- Result Parsers (exported for testing) ---

// QualifierVerdict represents the parsed qualifier agent result.
type QualifierVerdict struct {
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Severity   string  `json:"severity"`
}

// ParseQualifierResult parses the qualifier agent's JSON result.
// Returns a verdict with "needs_info" on parse failure.
func ParseQualifierResult(result string) QualifierVerdict {
	var v QualifierVerdict
	if err := json.Unmarshal([]byte(result), &v); err != nil {
		return QualifierVerdict{Verdict: PhaseNeedsInfo}
	}
	return v
}

// DeveloperOutput represents the parsed developer agent result.
type DeveloperOutput struct {
	BranchName   string   `json:"branch_name"`
	PRNumber     int      `json:"pr_number"`
	PRUrl        string   `json:"pr_url"`
	FilesChanged []string `json:"files_changed"`
}

// ParseDeveloperResult parses the developer agent's JSON result.
func ParseDeveloperResult(result string) DeveloperOutput {
	var o DeveloperOutput
	json.Unmarshal([]byte(result), &o) //nolint:errcheck // best-effort parse
	return o
}

// ReviewerOutput represents the parsed reviewer agent result.
type ReviewerOutput struct {
	Verdict  string `json:"verdict"`
	Feedback string `json:"feedback"`
}

// ParseReviewerResult parses the reviewer agent's JSON result.
func ParseReviewerResult(result string) ReviewerOutput {
	var o ReviewerOutput
	json.Unmarshal([]byte(result), &o) //nolint:errcheck // best-effort parse
	return o
}

// --- Discoverable interface ---

// Meta returns component metadata.
func (c *PRWorkflowComponent) Meta() component.Metadata {
	return component.Metadata{
		Name:        componentName,
		Type:        "processor",
		Description: "Spawns agent tasks for the GitHub issue-to-PR pipeline",
		Version:     componentVersion,
	}
}

// InputPorts returns the component's input port definitions.
func (c *PRWorkflowComponent) InputPorts() []component.Port {
	return c.inputPorts
}

// OutputPorts returns the component's output port definitions.
func (c *PRWorkflowComponent) OutputPorts() []component.Port {
	return c.outputPorts
}

// ConfigSchema returns the JSON schema for this component's configuration.
func (c *PRWorkflowComponent) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"model": {
				Type:        "string",
				Description: "Model endpoint name for agent tasks",
			},
			"token_budget": {
				Type:        "integer",
				Description: "Maximum tokens per workflow execution",
			},
			"max_review_cycles": {
				Type:        "integer",
				Description: "Maximum review rejection/retry loops",
			},
		},
	}
}

// Health returns the current health status of the component.
func (c *PRWorkflowComponent) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.running {
		return component.HealthStatus{Status: "healthy"}
	}
	return component.HealthStatus{Status: "stopped"}
}

// DataFlow returns current flow metrics for the component.
func (c *PRWorkflowComponent) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{}
}

// portSubject extracts the subject string from a port's config.
func portSubject(port component.Port) string {
	if port.Config == nil {
		return ""
	}
	switch cfg := port.Config.(type) {
	case component.NATSPort:
		return cfg.Subject
	case component.JetStreamPort:
		if len(cfg.Subjects) > 0 {
			return cfg.Subjects[0]
		}
	}
	return ""
}
