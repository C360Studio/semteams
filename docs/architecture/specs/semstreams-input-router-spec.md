# SemStreams: Input & Router Components Specification

**Version**: Draft v1  
**Status**: Planning  
**Location**: `processor/input/*`, `processor/agentic-dispatch/`

---

## Current State (What Already Exists)

Before implementation, note what infrastructure already exists in the codebase:

### Existing Agentic Infrastructure

| Component | Location | Status |
|-----------|----------|--------|
| `agentic-loop` | `processor/agentic-loop/` | ✓ Implemented |
| `agentic-model` | `processor/agentic-model/` | ✓ Implemented |
| `agentic-tools` | `processor/agentic-tools/` | ✓ Implemented |

### Existing NATS Subjects

| Subject Pattern | Publisher | Subscriber |
|-----------------|-----------|------------|
| `agent.task.*` | External | agentic-loop |
| `agent.request.{loopID}` | agentic-loop | agentic-model |
| `agent.response.{requestID}` | agentic-model | agentic-loop |
| `tool.execute.{toolName}` | agentic-loop | agentic-tools |
| `tool.result.{callID}` | agentic-tools | agentic-loop |
| `agent.complete.{loopID}` | agentic-loop | External |

### Existing Types (`agentic/types.go`)

- `TaskMessage` - Task submission (LoopID, TaskID, Role, Model, Prompt)
- `AgentRequest` / `AgentResponse` - Model communication
- `ToolCall` / `ToolResult` / `ToolDefinition` - Tool execution
- `ChatMessage` - Conversation messages
- `TokenUsage` - LLM token tracking

### Existing Loop States (`agentic/state.go`)

```go
// Current states - loops transition fluidly between non-terminal states
const (
    LoopStateExploring   = "exploring"   // Initial state
    LoopStatePlanning    = "planning"
    LoopStateArchitecting = "architecting"
    LoopStateExecuting   = "executing"
    LoopStateReviewing   = "reviewing"
    LoopStateComplete    = "complete"    // Terminal
    LoopStateFailed      = "failed"      // Terminal
)
```

### Existing KV Buckets

- `AGENT_LOOPS` - Loop entity state
- `AGENT_TRAJECTORIES` - Execution trajectory data

### Existing HTTP Patterns

```go
// service/openapi.go - Components implement this for HTTP endpoints
type HTTPHandler interface {
    RegisterHTTPHandlers(prefix string, mux *http.ServeMux)
    OpenAPISpec() *OpenAPISpec
}
```

ServiceManager coordinates HTTP with existing endpoints:
- `/health`, `/readyz`, `/metrics` - System endpoints
- `/components/*` - ComponentManager endpoints

### Existing Input Components

- `input/websocket/` - WebSocket server/client
- `input/udp/` - UDP network input
- `input/file/` - File reading

### What This Spec Adds (NEW)

| New Element | Description |
|-------------|-------------|
| `UserMessage`, `UserSignal`, `UserResponse` | New message types for user interaction |
| `user.message.*`, `user.signal.*`, `user.response.*` | New NATS subject patterns |
| `agent.signal.*` | New subject for loop control signals |
| `processor/agentic-dispatch/` | New routing component |
| `processor/input/cli/` | New CLI input component |
| Signal handling in agentic-loop | New functionality (cancel, pause, approve, etc.) |
| New loop states | `paused`, `cancelled`, `awaiting_approval` |
| `CommandRegistry`, `LoopTracker` | New infrastructure |
| Per-component HTTP endpoints | `/api/router/*`, `/api/agentic-loop/*`, etc. |

---

## Overview

The interaction layer bridges users and the agentic system. It normalizes input from various channels, parses commands, enforces permissions, routes tasks, and delivers responses.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                              │
│  USERS                      INTERACTION LAYER              AGENTIC          │
│                                                                              │
│  ┌─────────┐              ┌─────────────────┐                               │
│  │   CLI   │──┐           │                 │           ┌─────────────┐    │
│  └─────────┘  │           │                 │  agent.   │             │    │
│               │ user.     │   processor/    │  task.*   │  agentic-   │    │
│  ┌─────────┐  │ message.* │     router      │──────────►│    loop     │    │
│  │  Slack  │──┼──────────►│                 │           │             │    │
│  └─────────┘  │           │  • commands     │  agent.   └─────────────┘    │
│               │           │  • permissions  │  signal.*        │            │
│  ┌─────────┐  │ user.     │  • routing      │──────────►       │            │
│  │ Discord │──┤ signal.*  │  • responses    │                  │            │
│  └─────────┘  │──────────►│                 │◄─────────────────┘            │
│               │           │                 │  agent.complete.*             │
│  ┌─────────┐  │           └────────┬────────┘                               │
│  │   Web   │──┘                    │                                        │
│  └─────────┘              user.response.*                                   │
│       ▲                           │                                         │
│       └───────────────────────────┘                                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 1: Message & Signal Types

### UserMessage

Published to `user.message.{channel_type}.{channel_id}`

```go
// UserMessage represents normalized input from any channel
type UserMessage struct {
    // Identity
    MessageID   string `json:"message_id"`
    ChannelType string `json:"channel_type"`  // cli, slack, discord, web
    ChannelID   string `json:"channel_id"`    // specific conversation/channel
    UserID      string `json:"user_id"`
    
    // Content
    Content     string       `json:"content"`
    Attachments []Attachment `json:"attachments,omitempty"`
    
    // Context
    ReplyTo     string            `json:"reply_to,omitempty"`   // loop_id if continuing
    ThreadID    string            `json:"thread_id,omitempty"`  // for threaded channels
    Metadata    map[string]string `json:"metadata,omitempty"`   // channel-specific
    
    // Timing
    Timestamp   time.Time `json:"timestamp"`
}

type Attachment struct {
    Type     string `json:"type"`      // file, image, code, url
    Name     string `json:"name"`
    URL      string `json:"url,omitempty"`
    Content  string `json:"content,omitempty"`  // inline content if small
    MimeType string `json:"mime_type,omitempty"`
    Size     int64  `json:"size,omitempty"`
}
```

### UserSignal

Published to `user.signal.{loop_id}`

```go
// UserSignal represents a control signal from user
type UserSignal struct {
    SignalID    string    `json:"signal_id"`
    Type        string    `json:"type"`
    LoopID      string    `json:"loop_id"`
    UserID      string    `json:"user_id"`
    ChannelType string    `json:"channel_type"`
    ChannelID   string    `json:"channel_id"`
    Payload     any       `json:"payload,omitempty"`
    Timestamp   time.Time `json:"timestamp"`
}

// Signal types
const (
    SignalCancel   = "cancel"    // Stop execution immediately
    SignalPause    = "pause"     // Pause at next checkpoint
    SignalResume   = "resume"    // Continue paused loop
    SignalApprove  = "approve"   // Approve pending result
    SignalReject   = "reject"    // Reject with optional reason
    SignalFeedback = "feedback"  // Add feedback without decision
    SignalRetry    = "retry"     // Retry failed loop
)
```

### UserResponse

Published to `user.response.{channel_type}.{channel_id}`

```go
// UserResponse is sent back to users via their channel
type UserResponse struct {
    ResponseID  string `json:"response_id"`
    ChannelType string `json:"channel_type"`
    ChannelID   string `json:"channel_id"`
    UserID      string `json:"user_id"`     // who to respond to
    
    // What we're responding to
    InReplyTo   string `json:"in_reply_to,omitempty"`  // message_id or loop_id
    ThreadID    string `json:"thread_id,omitempty"`
    
    // Content
    Type        string `json:"type"`  // text, status, result, error, prompt
    Content     string `json:"content"`
    
    // Rich content (optional)
    Blocks      []ResponseBlock `json:"blocks,omitempty"`
    Actions     []ResponseAction `json:"actions,omitempty"`
    
    Timestamp   time.Time `json:"timestamp"`
}

type ResponseBlock struct {
    Type    string `json:"type"`     // text, code, diff, file, progress
    Content string `json:"content"`
    Lang    string `json:"lang,omitempty"`  // for code blocks
}

type ResponseAction struct {
    ID     string `json:"id"`
    Type   string `json:"type"`   // button, reaction
    Label  string `json:"label"`
    Signal string `json:"signal"` // signal to send if clicked
    Style  string `json:"style"`  // primary, danger, secondary
}
```

---

## Part 2: Input Components

### 2.1 processor/input/cli

**Purpose**: Normalize CLI input to UserMessage format

**Subscribes To**: stdin (direct), or `cli.input.*` for testing

**Publishes To**: `user.message.cli.*`, `user.signal.*`

```go
type CLIInput struct {
    config     Config
    natsClient *natsclient.Client
    userID     string  // configured user identity
    sessionID  string  // current CLI session
}

type Config struct {
    UserID       string `json:"user_id"`
    DefaultRole  string `json:"default_role"`
    Interactive  bool   `json:"interactive"`
    
    // Signal mappings
    CancelKey    string `json:"cancel_key"`  // default: Ctrl+C
    
    Ports PortConfig `json:"ports"`
}
```

**Behavior**:

```go
func (c *CLIInput) Run(ctx context.Context) error {
    // Set up signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt)  // Ctrl+C
    
    go func() {
        for range sigChan {
            if c.activeLoop != "" {
                c.publishSignal(SignalCancel, c.activeLoop)
            }
        }
    }()
    
    // Main input loop
    reader := bufio.NewReader(os.Stdin)
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            return err
        }
        
        msg := UserMessage{
            MessageID:   uuid.New().String(),
            ChannelType: "cli",
            ChannelID:   c.sessionID,
            UserID:      c.userID,
            Content:     strings.TrimSpace(line),
            Timestamp:   time.Now(),
        }
        
        c.publish(ctx, "user.message.cli."+c.sessionID, msg)
    }
}
```

**CLI-specific features**:
- Ctrl+C → cancel signal for active loop
- Ctrl+D → exit
- Direct stdin reading (not NATS-based input)
- Colorized output support

---

### 2.2 processor/input/slack (Optional)

**Purpose**: Bridge Slack events to UserMessage format

**External**: Slack Bolt/Socket Mode

**Publishes To**: `user.message.slack.*`, `user.signal.*`

```go
type SlackInput struct {
    config      Config
    boltApp     *slack.App
    natsClient  *natsclient.Client
}

type Config struct {
    BotToken    string `json:"bot_token"`
    AppToken    string `json:"app_token"`
    SigningSecret string `json:"signing_secret"`
    
    // Mapping Slack actions to signals
    ReactionMap map[string]string `json:"reaction_map"`  // emoji -> signal
    
    Ports PortConfig `json:"ports"`
}

// Default reaction mappings
var DefaultReactionMap = map[string]string{
    "white_check_mark": SignalApprove,  // ✅
    "x":                SignalReject,    // ❌
    "stop_sign":        SignalCancel,    // 🛑
    "pause_button":     SignalPause,     // ⏸️
    "arrow_forward":    SignalResume,    // ▶️
}
```

**Event handling**:

```go
func (s *SlackInput) handleMessage(evt *slackevents.MessageEvent) {
    // Skip bot messages
    if evt.BotID != "" {
        return
    }
    
    // Check for @mention or DM
    if !s.shouldProcess(evt) {
        return
    }
    
    msg := UserMessage{
        MessageID:   evt.ClientMsgID,
        ChannelType: "slack",
        ChannelID:   evt.Channel,
        UserID:      evt.User,
        Content:     s.stripMention(evt.Text),
        ThreadID:    evt.ThreadTimeStamp,
        Timestamp:   parseSlackTS(evt.TimeStamp),
        Metadata: map[string]string{
            "team_id": evt.Team,
            "ts":      evt.TimeStamp,
        },
    }
    
    // Handle file attachments
    for _, file := range evt.Files {
        msg.Attachments = append(msg.Attachments, Attachment{
            Type:     "file",
            Name:     file.Name,
            URL:      file.URLPrivate,
            MimeType: file.Mimetype,
            Size:     int64(file.Size),
        })
    }
    
    s.publish(ctx, "user.message.slack."+evt.Channel, msg)
}

func (s *SlackInput) handleReaction(evt *slackevents.ReactionAddedEvent) {
    signal, ok := s.config.ReactionMap[evt.Reaction]
    if !ok {
        return
    }
    
    // Find loop ID from message metadata
    loopID := s.findLoopForMessage(evt.Item.Timestamp)
    if loopID == "" {
        return
    }
    
    s.publishSignal(signal, loopID, evt.User)
}
```

---

### 2.3 processor/input/discord (Optional)

**Purpose**: Bridge Discord events to UserMessage format

**External**: Discord.js or discordgo

**Similar pattern to Slack** with Discord-specific:
- Slash commands: `/semspec add health endpoint`
- Button interactions for approve/reject
- Thread support

---

### 2.4 processor/input/web (Optional)

**Purpose**: HTTP/WebSocket API for web clients

**Endpoints**:
- `POST /api/message` → publish UserMessage
- `POST /api/signal` → publish UserSignal
- `WS /api/stream` → subscribe to UserResponse

---

## Part 3: Router Component

### processor/agentic-dispatch

**Purpose**: Parse commands, check permissions, route to handlers

**Subscribes To**: `user.message.>`, `agent.complete.*`

**Publishes To**: `agent.task.*`, `agent.signal.*`, `user.response.*`

### Configuration

```go
type Config struct {
    // Command definitions
    Commands map[string]CommandConfig `json:"commands"`
    
    // Permission rules
    Permissions PermissionConfig `json:"permissions"`
    
    // Routing defaults
    DefaultRole  string `json:"default_role"`
    DefaultModel string `json:"default_model"`
    
    // Behavior
    RequireExplicitLoop bool `json:"require_explicit_loop"`  // must specify loop_id
    AutoContinue        bool `json:"auto_continue"`          // continue last loop if exists
    
    // Rate limiting
    RateLimit RateLimitConfig `json:"rate_limit"`
    
    Ports PortConfig `json:"ports"`
}

type CommandConfig struct {
    Pattern     string   `json:"pattern"`       // regex pattern
    Handler     string   `json:"handler"`       // built-in handler name
    Signal      string   `json:"signal"`        // if it's a signal command
    RequireLoop bool     `json:"require_loop"`  // must have active loop
    Permission  string   `json:"permission"`    // required permission
    Help        string   `json:"help"`          // help text
}

type PermissionConfig struct {
    // Basic permissions
    View       []string `json:"view"`         // who can view status, loops, history
    SubmitTask []string `json:"submit_task"`  // who can submit new tasks
    
    // Signal permissions
    CancelOwn  bool     `json:"cancel_own"`   // users can cancel their own loops
    CancelAny  []string `json:"cancel_any"`   // who can cancel any loop
    PauseAny   []string `json:"pause_any"`
    
    // Approval permissions
    Approve    []string `json:"approve"`       // who can approve results
    
    // System visibility
    ViewSystem []string `json:"view_system"`   // who can use /ps, /top, /system
    
    // Role restrictions
    Roles map[string][]string `json:"roles"`  // role -> allowed users
    
    // Channel restrictions
    Channels map[string][]string `json:"channels"`  // channel_id -> allowed users
}

// Permission summary:
// - view:        /status, /loops, /history, /help, /feedback
// - submit_task: submit new tasks (messages without /)
// - cancel_own:  /cancel, /pause, /resume (own loops)
// - cancel_any:  /kill (any loop)
// - approve:     /approve, /reject
// - view_system: /ps, /top, /system

type RateLimitConfig struct {
    Enabled       bool          `json:"enabled"`
    RequestsPerMin int          `json:"requests_per_min"`
    BurstSize     int           `json:"burst_size"`
    PerUser       bool          `json:"per_user"`
}
```

### Default Commands

Commands are organized by scope: **user** (your loops) and **system** (all loops/components).

```json
{
  "commands": {
    "cancel": {
      "pattern": "^/cancel\\s*(\\S+)?$",
      "handler": "signal",
      "signal": "cancel",
      "require_loop": false,
      "permission": "cancel_own",
      "scope": "user",
      "help": "/cancel [loop_id] - Cancel current or specified loop"
    },
    "status": {
      "pattern": "^/status\\s*(\\S+)?$",
      "handler": "status",
      "require_loop": false,
      "permission": "view",
      "scope": "user",
      "help": "/status [loop_id] - Show loop status"
    },
    "pause": {
      "pattern": "^/pause\\s*(\\S+)?$",
      "handler": "signal",
      "signal": "pause",
      "require_loop": true,
      "permission": "cancel_own",
      "scope": "user",
      "help": "/pause [loop_id] - Pause at next checkpoint"
    },
    "resume": {
      "pattern": "^/resume\\s*(\\S+)?$",
      "handler": "signal",
      "signal": "resume",
      "require_loop": true,
      "permission": "cancel_own",
      "scope": "user",
      "help": "/resume [loop_id] - Resume paused loop"
    },
    "approve": {
      "pattern": "^/approve\\s*(\\S+)?$",
      "handler": "signal",
      "signal": "approve",
      "require_loop": true,
      "permission": "approve",
      "scope": "user",
      "help": "/approve [loop_id] - Approve pending result"
    },
    "reject": {
      "pattern": "^/reject\\s*(\\S+)?\\s*(.*)$",
      "handler": "signal",
      "signal": "reject",
      "require_loop": true,
      "permission": "approve",
      "scope": "user",
      "help": "/reject [loop_id] [reason] - Reject with reason"
    },
    "feedback": {
      "pattern": "^/feedback\\s+(\\S+)\\s+(.+)$",
      "handler": "signal",
      "signal": "feedback",
      "require_loop": true,
      "permission": "view",
      "scope": "user",
      "help": "/feedback <loop_id> <text> - Add feedback"
    },
    "loops": {
      "pattern": "^/loops$",
      "handler": "list_loops",
      "require_loop": false,
      "permission": "view",
      "scope": "user",
      "help": "/loops - List your active loops"
    },
    "history": {
      "pattern": "^/history\\s*(\\d+)?$",
      "handler": "history",
      "require_loop": false,
      "permission": "view",
      "scope": "user",
      "help": "/history [n] - Show recent loop history"
    },
    "ps": {
      "pattern": "^/ps$",
      "handler": "process_status",
      "require_loop": false,
      "permission": "view_system",
      "scope": "system",
      "help": "/ps - List ALL active loops (like Unix ps)"
    },
    "top": {
      "pattern": "^/top$",
      "handler": "activity_stream",
      "require_loop": false,
      "permission": "view_system",
      "scope": "system",
      "help": "/top - Live activity feed (like Unix top)"
    },
    "system": {
      "pattern": "^/system$",
      "handler": "system_status",
      "require_loop": false,
      "permission": "view_system",
      "scope": "system",
      "help": "/system - Component health and stats"
    },
    "kill": {
      "pattern": "^/kill\\s+(\\S+)$",
      "handler": "signal",
      "signal": "cancel",
      "require_loop": true,
      "permission": "cancel_any",
      "scope": "system",
      "help": "/kill <loop_id> - Force cancel any loop (admin)"
    },
    "help": {
      "pattern": "^/help$",
      "handler": "help",
      "require_loop": false,
      "permission": "",
      "scope": "user",
      "help": "/help - Show available commands"
    }
  }
}
```

### Command Summary

**User Commands** (your loops):

| Command | Description | Permission |
|---------|-------------|------------|
| `/status [id]` | Show loop status | `view` |
| `/loops` | List your active loops | `view` |
| `/history [n]` | Recent loop history | `view` |
| `/cancel [id]` | Cancel your loop | `cancel_own` |
| `/pause [id]` | Pause at checkpoint | `cancel_own` |
| `/resume [id]` | Resume paused loop | `cancel_own` |
| `/approve [id]` | Approve result | `approve` |
| `/reject [id] [reason]` | Reject with reason | `approve` |
| `/feedback <id> <text>` | Add feedback | `view` |
| `/help` | Show commands | (none) |

**System Commands** (all loops, operators):

| Command | Description | Permission |
|---------|-------------|------------|
| `/ps` | List ALL active loops | `view_system` |
| `/top` | Live activity stream | `view_system` |
| `/system` | Component health | `view_system` |
| `/kill <id>` | Force cancel any loop | `cancel_any` |

### System Command Output Examples

**`/ps` - Process Status**:
```
LOOP         STATE       OWNER      SOURCE         AGE     ITER  
abc123       executing   coby       cli            2m      3/20
def456       executing   system     filewatcher    45s     1/5
ghi789       paused      system     webhook        10m     2/10
jkl012       exploring   scheduler  cron           1h      1/20

4 active loops (2 user, 2 system)
```

**`/top` - Live Activity**:
```
[10:30:15] loop:def456 [filewatcher] tool:ast_parse gateway/server.go
[10:30:14] loop:abc123 [cli:coby] model:response 847 tokens
[10:30:12] loop:def456 [filewatcher] tool:ast_parse gateway/health.go
[10:30:10] loop:abc123 [cli:coby] tool:file_read auth/token.go

Press q to exit, c to cancel a loop, p to pause
```

**`/system` - System Health**:
```
COMPONENTS
  router         ✓ running   uptime: 2h 15m
  agentic-loop   ✓ running   uptime: 2h 15m
  agentic-model  ✓ running   uptime: 2h 15m   ollama: connected
  agentic-tools  ✓ running   uptime: 2h 15m
  ast-processor  ✓ running   uptime: 2h 15m   entities: 847
  filewatcher    ✓ running   uptime: 2h 15m   watching: 3 paths

NATS
  streams: 4 (AGENT, USER, TOOL, ENTITY)
  consumers: 12 active
  messages: 1,247 (last hour)

RESOURCES
  active loops: 4
  pending tools: 2
  model queue: 0
```

### Router Implementation

```go
type Router struct {
    config      Config
    natsClient  *natsclient.Client
    loopTracker *LoopTracker  // tracks active loops per user/channel
    rateLimiter *RateLimiter
    logger      *slog.Logger
}

// LoopTracker maintains user -> active loop mapping
type LoopTracker struct {
    mu sync.RWMutex
    
    // user_id -> loop_id (most recent)
    userLoops map[string]string
    
    // channel_id -> loop_id (for channel-scoped)
    channelLoops map[string]string
    
    // loop_id -> LoopInfo
    loops map[string]*LoopInfo
}

type LoopInfo struct {
    LoopID        string
    UserID        string     // "coby" or "system"
    Source        string     // "cli", "filewatcher", "webhook", "scheduler"
    SourceID      string     // specific instance identifier
    ChannelType   string
    ChannelID     string
    State         string
    Iterations    int
    MaxIterations int
    CreatedAt     time.Time
}
```

### Message Handling

```go
func (r *Router) HandleMessage(ctx context.Context, msg UserMessage) error {
    // 1. Rate limiting
    if r.config.RateLimit.Enabled {
        if !r.rateLimiter.Allow(msg.UserID) {
            return r.sendRateLimitError(ctx, msg)
        }
    }
    
    // 2. Check if it's a command
    if strings.HasPrefix(msg.Content, "/") {
        return r.handleCommand(ctx, msg)
    }
    
    // 3. Check submit permission
    if !r.hasPermission(msg.UserID, "submit_task") {
        return r.sendPermissionError(ctx, msg, "submit_task")
    }
    
    // 4. Determine if continuing existing loop or starting new
    var loopID string
    if msg.ReplyTo != "" {
        loopID = msg.ReplyTo
    } else if r.config.AutoContinue {
        loopID = r.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
    }
    
    // 5. Build and publish task
    task := r.buildTask(msg, loopID)
    
    // 6. Track the loop
    r.loopTracker.Track(task.LoopID, &LoopInfo{
        LoopID:      task.LoopID,
        UserID:      msg.UserID,
        ChannelType: msg.ChannelType,
        ChannelID:   msg.ChannelID,
        State:       "pending",
        CreatedAt:   time.Now(),
    })
    
    // 7. Send acknowledgment
    r.sendAck(ctx, msg, task.LoopID)
    
    // 8. Publish task
    return r.publish(ctx, "agent.task."+task.TaskID, task)
}

func (r *Router) buildTask(msg UserMessage, existingLoopID string) TaskMessage {
    taskID := uuid.New().String()
    
    loopID := existingLoopID
    if loopID == "" {
        loopID = "loop_" + uuid.New().String()[:8]
    }
    
    return TaskMessage{
        TaskID:  taskID,
        LoopID:  loopID,
        Role:    r.config.DefaultRole,
        Model:   r.config.DefaultModel,
        Prompt:  msg.Content,
        
        // Preserve context
        Metadata: map[string]string{
            "user_id":      msg.UserID,
            "channel_type": msg.ChannelType,
            "channel_id":   msg.ChannelID,
            "message_id":   msg.MessageID,
        },
    }
}
```

### Command Handling

```go
func (r *Router) handleCommand(ctx context.Context, msg UserMessage) error {
    content := msg.Content
    
    for name, cmdConfig := range r.config.Commands {
        re := regexp.MustCompile(cmdConfig.Pattern)
        matches := re.FindStringSubmatch(content)
        if matches == nil {
            continue
        }
        
        // Check permission
        if cmdConfig.Permission != "" && !r.hasPermission(msg.UserID, cmdConfig.Permission) {
            return r.sendPermissionError(ctx, msg, cmdConfig.Permission)
        }
        
        // Resolve loop ID
        loopID := ""
        if len(matches) > 1 && matches[1] != "" {
            loopID = matches[1]
        } else {
            loopID = r.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
        }
        
        // Check if loop required
        if cmdConfig.RequireLoop && loopID == "" {
            return r.sendError(ctx, msg, "No active loop. Specify a loop_id or start a task first.")
        }
        
        // Check loop ownership for cancel_own permission
        if cmdConfig.Permission == "cancel_own" && loopID != "" {
            if !r.canUserControlLoop(msg.UserID, loopID) {
                return r.sendPermissionError(ctx, msg, "cancel_any")
            }
        }
        
        // Handle based on handler type
        switch cmdConfig.Handler {
        case "signal":
            return r.handleSignalCommand(ctx, msg, cmdConfig.Signal, loopID, matches)
        case "status":
            return r.handleStatusCommand(ctx, msg, loopID)
        case "list_loops":
            return r.handleListLoopsCommand(ctx, msg)
        case "history":
            return r.handleHistoryCommand(ctx, msg, matches)
        case "help":
            return r.handleHelpCommand(ctx, msg)
        // System commands
        case "process_status":
            return r.handlePsCommand(ctx, msg)
        case "activity_stream":
            return r.handleTopCommand(ctx, msg)
        case "system_status":
            return r.handleSystemCommand(ctx, msg)
        default:
            return r.sendError(ctx, msg, "Unknown command handler: "+cmdConfig.Handler)
        }
    }
    
    return r.sendError(ctx, msg, "Unknown command. Type /help for available commands.")
}

// System command handlers

func (r *Router) handlePsCommand(ctx context.Context, msg UserMessage) error {
    loops := r.loopTracker.GetAllLoops()
    
    var lines []string
    lines = append(lines, "LOOP         STATE       OWNER      SOURCE         AGE     ITER")
    
    userCount, systemCount := 0, 0
    for _, loop := range loops {
        age := time.Since(loop.CreatedAt).Truncate(time.Second)
        iter := fmt.Sprintf("%d/%d", loop.Iterations, loop.MaxIterations)
        
        lines = append(lines, fmt.Sprintf("%-12s %-11s %-10s %-14s %-7s %s",
            loop.LoopID[:8]+"...",
            loop.State,
            loop.UserID,
            loop.Source,
            formatAge(age),
            iter,
        ))
        
        if loop.UserID == "system" {
            systemCount++
        } else {
            userCount++
        }
    }
    
    lines = append(lines, "")
    lines = append(lines, fmt.Sprintf("%d active loops (%d user, %d system)", 
        len(loops), userCount, systemCount))
    
    return r.sendResponse(ctx, msg, UserResponse{
        Type:    "text",
        Content: strings.Join(lines, "\n"),
    })
}

func (r *Router) handleTopCommand(ctx context.Context, msg UserMessage) error {
    // Subscribe to activity and stream to user
    // This is a long-running command that streams output
    
    activityCh := make(chan ActivityEvent, 100)
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()
    
    // Subscribe to activity subjects
    r.subscribeActivity(ctx, activityCh)
    
    // Send initial message
    r.sendResponse(ctx, msg, UserResponse{
        Type:    "status",
        Content: "Live activity (press Ctrl+C to exit):\n",
    })
    
    // Stream activity
    for {
        select {
        case event := <-activityCh:
            line := fmt.Sprintf("[%s] loop:%s [%s] %s",
                event.Timestamp.Format("15:04:05"),
                event.LoopID[:8],
                event.Source,
                event.Description,
            )
            r.sendResponse(ctx, msg, UserResponse{
                Type:    "stream",
                Content: line + "\n",
            })
        case <-ctx.Done():
            return nil
        }
    }
}

func (r *Router) handleSystemCommand(ctx context.Context, msg UserMessage) error {
    // Gather system status from components
    status := r.gatherSystemStatus(ctx)
    
    var lines []string
    
    // Components section
    lines = append(lines, "COMPONENTS")
    for _, comp := range status.Components {
        statusIcon := "✓"
        if comp.Status != "running" {
            statusIcon = "✗"
        }
        line := fmt.Sprintf("  %-14s %s %-8s uptime: %s",
            comp.Name, statusIcon, comp.Status, formatAge(comp.Uptime))
        if comp.Extra != "" {
            line += "   " + comp.Extra
        }
        lines = append(lines, line)
    }
    
    // NATS section
    lines = append(lines, "")
    lines = append(lines, "NATS")
    lines = append(lines, fmt.Sprintf("  streams: %d (%s)", 
        status.NATS.StreamCount, strings.Join(status.NATS.StreamNames, ", ")))
    lines = append(lines, fmt.Sprintf("  consumers: %d active", status.NATS.ConsumerCount))
    lines = append(lines, fmt.Sprintf("  messages: %d (last hour)", status.NATS.MessagesLastHour))
    
    // Resources section
    lines = append(lines, "")
    lines = append(lines, "RESOURCES")
    lines = append(lines, fmt.Sprintf("  active loops: %d", status.ActiveLoops))
    lines = append(lines, fmt.Sprintf("  pending tools: %d", status.PendingTools))
    lines = append(lines, fmt.Sprintf("  model queue: %d", status.ModelQueue))
    
    return r.sendResponse(ctx, msg, UserResponse{
        Type:    "text",
        Content: strings.Join(lines, "\n"),
    })
}

func (r *Router) handleSignalCommand(ctx context.Context, msg UserMessage, signalType, loopID string, matches []string) error {
    signal := UserSignal{
        SignalID:    uuid.New().String(),
        Type:        signalType,
        LoopID:      loopID,
        UserID:      msg.UserID,
        ChannelType: msg.ChannelType,
        ChannelID:   msg.ChannelID,
        Timestamp:   time.Now(),
    }
    
    // Extract payload for signals that need it
    switch signalType {
    case SignalReject:
        if len(matches) > 2 {
            signal.Payload = matches[2]  // rejection reason
        }
    case SignalFeedback:
        if len(matches) > 2 {
            signal.Payload = matches[2]  // feedback text
        }
    }
    
    // Publish signal
    if err := r.publish(ctx, "agent.signal."+loopID, signal); err != nil {
        return err
    }
    
    // Acknowledge
    return r.sendResponse(ctx, msg, UserResponse{
        Type:    "status",
        Content: fmt.Sprintf("Signal '%s' sent to loop %s", signalType, loopID),
    })
}
```

### Completion Handling

```go
func (r *Router) HandleCompletion(ctx context.Context, completion CompletionEvent) error {
    // Look up original channel info
    loopInfo := r.loopTracker.Get(completion.LoopID)
    if loopInfo == nil {
        r.logger.Warn("No tracking info for completed loop", "loop_id", completion.LoopID)
        return nil
    }
    
    // Build response based on outcome
    var response UserResponse
    switch completion.Outcome {
    case "complete":
        response = r.buildCompletionResponse(completion, loopInfo)
    case "cancelled":
        response = UserResponse{
            Type:    "status",
            Content: fmt.Sprintf("Loop %s cancelled.", completion.LoopID),
        }
    case "failed":
        response = UserResponse{
            Type:    "error",
            Content: fmt.Sprintf("Loop %s failed: %s", completion.LoopID, completion.Error),
        }
    case "awaiting_approval":
        response = r.buildApprovalPrompt(completion, loopInfo)
    }
    
    // Route to appropriate channel
    response.ChannelType = loopInfo.ChannelType
    response.ChannelID = loopInfo.ChannelID
    response.UserID = loopInfo.UserID
    response.InReplyTo = completion.LoopID
    
    subject := fmt.Sprintf("user.response.%s.%s", loopInfo.ChannelType, loopInfo.ChannelID)
    return r.publish(ctx, subject, response)
}

func (r *Router) buildApprovalPrompt(completion CompletionEvent, loopInfo *LoopInfo) UserResponse {
    return UserResponse{
        Type:    "prompt",
        Content: fmt.Sprintf("Loop %s ready for review.", completion.LoopID),
        Blocks: []ResponseBlock{
            {Type: "text", Content: "Changes:"},
            {Type: "diff", Content: completion.Diff},
        },
        Actions: []ResponseAction{
            {ID: "approve", Type: "button", Label: "Approve", Signal: SignalApprove, Style: "primary"},
            {ID: "reject", Type: "button", Label: "Reject", Signal: SignalReject, Style: "danger"},
        },
    }
}
```

### Permission Checking

```go
func (r *Router) hasPermission(userID, permission string) bool {
    switch permission {
    case "view":
        return r.inList(userID, r.config.Permissions.View)
    case "submit_task":
        return r.inList(userID, r.config.Permissions.SubmitTask)
    case "cancel_own":
        return r.config.Permissions.CancelOwn
    case "cancel_any":
        return r.inList(userID, r.config.Permissions.CancelAny)
    case "pause_any":
        return r.inList(userID, r.config.Permissions.PauseAny)
    case "approve":
        return r.inList(userID, r.config.Permissions.Approve)
    case "view_system":
        return r.inList(userID, r.config.Permissions.ViewSystem)
    default:
        return false
    }
}

func (r *Router) canUserControlLoop(userID, loopID string) bool {
    // Can always control if has cancel_any
    if r.inList(userID, r.config.Permissions.CancelAny) {
        return true
    }
    
    // Check if user owns the loop
    loopInfo := r.loopTracker.Get(loopID)
    if loopInfo == nil {
        return false
    }
    
    return loopInfo.UserID == userID && r.config.Permissions.CancelOwn
}

func (r *Router) inList(userID string, list []string) bool {
    for _, allowed := range list {
        if allowed == "*" || allowed == userID {
            return true
        }
    }
    return false
}
```

---

## Part 4: Signal Handling in agentic-loop

> **NOTE**: This is entirely NEW functionality. The current agentic-loop has no signal handling.
> All code in this section must be added to the existing component.

Updates needed to `processor/agentic-loop`:

### New States (add to `agentic/state.go`)

```go
// agentic/state.go - ADD these to existing states
const (
    // EXISTING states (already implemented):
    // LoopStateExploring   = "exploring"
    // LoopStatePlanning    = "planning"
    // LoopStateArchitecting = "architecting"
    // LoopStateExecuting   = "executing"
    // LoopStateReviewing   = "reviewing"
    // LoopStateComplete    = "complete"    // Terminal
    // LoopStateFailed      = "failed"      // Terminal
    
    // NEW states for signal support:
    LoopStatePaused           LoopState = "paused"            // Paused by user signal
    LoopStateCancelled        LoopState = "cancelled"         // Cancelled by user signal (terminal)
    LoopStateAwaitingApproval LoopState = "awaiting_approval" // Awaiting user approval
)
```

### Signal Subscription (add to `component.go`)

```go
// processor/agentic-loop/component.go - ADD to setupSubscriptions()
func (c *Component) setupSubscriptions(ctx context.Context) error {
    // EXISTING subscriptions for agent.task.*, agent.response.>, tool.result.> ...
    
    // NEW: Add signal handling subscription
    _, err = c.natsClient.Subscribe(ctx, "agent.signal.*", func(ctx context.Context, data []byte) {
        c.handleSignalMessage(ctx, data)
    })
    return err
}

func (c *Component) handleSignalMessage(ctx context.Context, data []byte) {
    var signal UserSignal
    if err := json.Unmarshal(data, &signal); err != nil {
        c.logger.Error("Failed to unmarshal signal", "error", err)
        return
    }
    
    switch signal.Type {
    case SignalCancel:
        c.handleCancel(ctx, signal)
    case SignalPause:
        c.handlePause(ctx, signal)
    case SignalResume:
        c.handleResume(ctx, signal)
    case SignalApprove:
        c.handleApprove(ctx, signal)
    case SignalReject:
        c.handleReject(ctx, signal)
    case SignalFeedback:
        c.handleFeedback(ctx, signal)
    case SignalRetry:
        c.handleRetry(ctx, signal)
    }
}
```

### Signal Handlers

```go
func (c *Component) handleCancel(ctx context.Context, signal UserSignal) {
    entity, err := c.getLoop(signal.LoopID)
    if err != nil {
        c.logger.Error("Cancel: loop not found", "loop_id", signal.LoopID)
        return
    }
    
    // Check if already terminal
    if entity.State == agentic.LoopStateComplete || 
       entity.State == agentic.LoopStateFailed ||
       entity.State == agentic.LoopStateCancelled {
        return
    }
    
    // Update state
    entity.State = agentic.LoopStateCancelled
    entity.CancelledBy = signal.UserID
    entity.CancelledAt = time.Now()
    c.persistLoopState(ctx, signal.LoopID)
    
    // Cancel pending tool calls
    for _, toolCallID := range entity.PendingTools {
        c.publish(ctx, "tool.cancel."+toolCallID, map[string]string{
            "call_id": toolCallID,
            "reason":  "user_cancelled",
        })
    }
    
    // Finalize trajectory
    c.finalizeTrajectory(ctx, signal.LoopID, "cancelled")
    
    // Publish completion
    c.publish(ctx, "agent.complete."+signal.LoopID, CompletionEvent{
        LoopID:  signal.LoopID,
        Outcome: "cancelled",
        Reason:  fmt.Sprintf("Cancelled by user %s", signal.UserID),
    })
}

func (c *Component) handlePause(ctx context.Context, signal UserSignal) {
    entity, err := c.getLoop(signal.LoopID)
    if err != nil {
        return
    }
    
    // Can only pause active loops
    if entity.State == agentic.LoopStateComplete ||
       entity.State == agentic.LoopStateFailed ||
       entity.State == agentic.LoopStateCancelled ||
       entity.State == agentic.LoopStatePaused {
        return
    }
    
    // Mark as paused (will take effect at next checkpoint)
    entity.PauseRequested = true
    entity.PauseRequestedBy = signal.UserID
    c.persistLoopState(ctx, signal.LoopID)
}

func (c *Component) handleResume(ctx context.Context, signal UserSignal) {
    entity, err := c.getLoop(signal.LoopID)
    if err != nil {
        return
    }
    
    if entity.State != agentic.LoopStatePaused {
        return
    }
    
    // Resume from paused state
    entity.State = entity.StateBeforePause
    entity.PauseRequested = false
    c.persistLoopState(ctx, signal.LoopID)
    
    // Continue execution - republish pending request
    c.continueLoop(ctx, signal.LoopID)
}

func (c *Component) handleApprove(ctx context.Context, signal UserSignal) {
    entity, err := c.getLoop(signal.LoopID)
    if err != nil {
        return
    }
    
    if entity.State != agentic.LoopStateAwaitingApproval {
        return
    }
    
    // Mark result as approved
    result := c.getResult(signal.LoopID)
    result.Status = "approved"
    result.ApprovedBy = signal.UserID
    result.ApprovedAt = time.Now()
    c.persistResult(ctx, result)
    
    // Update loop state
    entity.State = agentic.LoopStateComplete
    c.persistLoopState(ctx, signal.LoopID)
    
    // Mark trajectory as training-eligible
    c.markTrajectoryEligible(ctx, signal.LoopID)
    
    // Publish completion
    c.publish(ctx, "agent.complete."+signal.LoopID, CompletionEvent{
        LoopID:  signal.LoopID,
        Outcome: "approved",
    })
}

func (c *Component) handleReject(ctx context.Context, signal UserSignal) {
    entity, err := c.getLoop(signal.LoopID)
    if err != nil {
        return
    }
    
    if entity.State != agentic.LoopStateAwaitingApproval {
        return
    }
    
    // Mark result as rejected
    result := c.getResult(signal.LoopID)
    result.Status = "rejected"
    result.RejectedBy = signal.UserID
    result.RejectedAt = time.Now()
    if signal.Payload != nil {
        result.RejectionReason = signal.Payload.(string)
    }
    c.persistResult(ctx, result)
    
    // Update loop - goes back to executing for revision
    entity.State = agentic.LoopStateExecuting
    c.persistLoopState(ctx, signal.LoopID)
    
    // Continue with rejection feedback
    c.continueWithFeedback(ctx, signal.LoopID, result.RejectionReason)
}

func (c *Component) handleFeedback(ctx context.Context, signal UserSignal) {
    // Store feedback without changing state
    feedback := Feedback{
        LoopID:    signal.LoopID,
        UserID:    signal.UserID,
        Content:   signal.Payload.(string),
        Timestamp: signal.Timestamp,
    }
    c.storeFeedback(ctx, feedback)
}
```

---

## Part 5: Configuration Examples

### Minimal (CLI only, single user)

```json
{
  "router": {
    "default_role": "general",
    "default_model": "qwen2.5-coder:32b",
    "permissions": {
      "view": ["*"],
      "submit_task": ["*"],
      "cancel_own": true,
      "approve": ["*"],
      "view_system": ["*"]
    }
  },
  "inputs": {
    "cli": {
      "enabled": true,
      "user_id": "coby"
    }
  }
}
```

### Multi-channel with restrictions

```json
{
  "router": {
    "default_role": "general",
    "default_model": "qwen2.5-coder:32b",
    "permissions": {
      "view": ["*"],
      "submit_task": ["coby", "team-lead", "alice", "bob"],
      "cancel_own": true,
      "cancel_any": ["coby"],
      "approve": ["coby", "team-lead"],
      "view_system": ["coby", "team-lead"],
      "roles": {
        "architect": ["coby"],
        "implementer": ["*"]
      }
    },
    "rate_limit": {
      "enabled": true,
      "requests_per_min": 10,
      "per_user": true
    }
  },
  "inputs": {
    "cli": {
      "enabled": true,
      "user_id": "coby"
    },
    "slack": {
      "enabled": true,
      "bot_token": "${SLACK_BOT_TOKEN}",
      "app_token": "${SLACK_APP_TOKEN}",
      "reaction_map": {
        "white_check_mark": "approve",
        "x": "reject",
        "octagonal_sign": "cancel"
      }
    }
  }
}
```

### Role-based configuration

```json
{
  "router": {
    "permissions": {
      "roles": {
        "admin": {
          "permissions": ["*"]
        },
        "operator": {
          "permissions": ["view", "submit_task", "cancel_own", "view_system"]
        },
        "developer": {
          "permissions": ["view", "submit_task", "cancel_own"]
        },
        "reviewer": {
          "permissions": ["view", "approve"]
        }
      },
      "users": {
        "coby": "admin",
        "alice": "operator",
        "bob": "developer",
        "carol": "reviewer"
      }
    }
  }
}
```

---

## Part 6: Command Registry

Applications extend the router by registering custom commands, following the same pattern as tool registration
in the agentic-tools component.

### Two Registration Approaches

Commands can be registered in two ways:

1. **Global registration via init()** - Preferred for reusable commands
2. **Per-component registration** - For component-specific commands

The router loads global commands automatically during initialization, after built-in commands are registered.

### Global Command Registration

External packages can register commands globally using `RegisterCommand()` in an `init()` function:

```go
// router/global.go

// RegisterCommand registers a command executor globally via init().
// Returns an error if the command name is empty or already registered.
// Panics if executor is nil (programmer error).
func RegisterCommand(name string, executor CommandExecutor) error

// ListRegisteredCommands returns a copy of all globally registered commands.
func ListRegisteredCommands() map[string]CommandExecutor

// CommandContext provides services to command executors
type CommandContext struct {
    NATSClient    *natsclient.Client
    LoopTracker   *LoopTracker
    Logger        *slog.Logger
    HasPermission func(userID, permission string) bool
}

// CommandExecutor is the interface for command implementations
type CommandExecutor interface {
    Execute(ctx context.Context, cmdCtx *CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error)
    Config() CommandConfig
}
```

**Example: Global registration in external package**

```go
// semspec/commands/spec.go
package commands

import (
    "context"
    "github.com/c360/semstreams/agentic"
    "agenticdispatch "github.com/c360/semstreams/processor/agentic-dispatch""
)

func init() {
    if err := router.RegisterCommand("spec", &SpecCommand{}); err != nil {
        panic(err) // Registration errors are programmer errors
    }
}

type SpecCommand struct{}

func (c *SpecCommand) Config() router.CommandConfig {
    return router.CommandConfig{
        Pattern:     `^/spec\s*(.*)$`,
        Permission:  "submit_task",
        RequireLoop: false,
        Scope:       "user",
        Category:    "semspec",
        Help:        "/spec [name] - Run spec-driven development workflow",
    }
}

func (c *SpecCommand) Execute(ctx context.Context, cmdCtx *router.CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
    // Access router services via cmdCtx:
    // - cmdCtx.NATSClient for publishing messages
    // - cmdCtx.LoopTracker for active loop tracking
    // - cmdCtx.Logger for logging
    // - cmdCtx.HasPermission(userID, perm) for permission checks

    specName := ""
    if len(args) > 0 && args[0] != "" {
        specName = args[0]
    }

    // Example: publish a task to start a spec workflow
    taskMsg := agentic.TaskMessage{
        LoopID: "spec_" + uuid.New().String()[:8],
        TaskID: uuid.New().String(),
        Role:   "architect",
        Model:  "qwen2.5-coder:32b",
        Prompt: fmt.Sprintf("Create specification: %s", specName),
    }

    data, _ := json.Marshal(taskMsg)
    if err := cmdCtx.NATSClient.Publish(ctx, "agent.task."+taskMsg.TaskID, data); err != nil {
        return agentic.UserResponse{}, err
    }

    return agentic.UserResponse{
        Type:    "status",
        Content: fmt.Sprintf("Started spec workflow: %s", taskMsg.LoopID),
    }, nil
}
```

### Per-Component Registration

The CommandRegistry also supports per-component registration for component-specific commands:

```go
// router/command_registry.go

// CommandRegistry manages command registration
type CommandRegistry struct {
    mu       sync.RWMutex
    commands map[string]RegisteredCommand
}

type RegisteredCommand struct {
    Config  CommandConfig
    Handler CommandHandler
}

// CommandHandler processes a command and returns a response
type CommandHandler func(ctx context.Context, msg UserMessage, args []string) (UserResponse, error)

func NewCommandRegistry() *CommandRegistry {
    return &CommandRegistry{
        commands: make(map[string]RegisteredCommand),
    }
}

// Register adds a command with its handler
func (r *CommandRegistry) Register(name string, config CommandConfig, handler CommandHandler) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if _, exists := r.commands[name]; exists {
        return fmt.Errorf("command %s already registered", name)
    }

    // Compile pattern
    if config.Pattern != "" {
        config.compiledPattern = regexp.MustCompile(config.Pattern)
    }

    r.commands[name] = RegisteredCommand{
        Config:  config,
        Handler: handler,
    }
    return nil
}

// Get retrieves a registered command
func (r *CommandRegistry) Get(name string) (RegisteredCommand, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    cmd, ok := r.commands[name]
    return cmd, ok
}

// All returns all registered commands (for /help)
func (r *CommandRegistry) All() map[string]CommandConfig {
    r.mu.RLock()
    defer r.mu.RUnlock()
    result := make(map[string]CommandConfig, len(r.commands))
    for name, reg := range r.commands {
        result[name] = reg.Config
    }
    return result
}

// Match finds a command matching the input
func (r *CommandRegistry) Match(input string) (string, RegisteredCommand, []string, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for name, reg := range r.commands {
        if reg.Config.compiledPattern != nil {
            if matches := reg.Config.compiledPattern.FindStringSubmatch(input); matches != nil {
                return name, reg, matches[1:], true  // return captured groups
            }
        }
    }
    return "", RegisteredCommand{}, nil, false
}
```

### Router Integration

```go
// router/router.go

type Router struct {
    config     Config
    registry   *CommandRegistry
    natsClient *natsclient.Client
    // ...
}

func NewRouter(config Config, nats *natsclient.Client) *Router {
    r := &Router{
        config:     config,
        registry:   NewCommandRegistry(),
        natsClient: nats,
    }

    // Register built-in commands first
    r.registerBuiltinCommands()

    // Load globally registered commands via init()
    r.loadGlobalCommands()

    return r
}

// loadGlobalCommands registers all globally registered commands
func (r *Router) loadGlobalCommands() {
    globalCommands := ListRegisteredCommands()
    for name, executor := range globalCommands {
        config := executor.Config()
        handler := r.makeHandlerFromExecutor(executor)
        if err := r.registry.Register(name, config, handler); err != nil {
            r.logger.Error("failed to register global command",
                "command", name,
                "error", err)
        }
    }
}

// makeHandlerFromExecutor adapts a CommandExecutor to CommandHandler
func (r *Router) makeHandlerFromExecutor(executor CommandExecutor) CommandHandler {
    return func(ctx context.Context, msg UserMessage, args []string) (UserResponse, error) {
        cmdCtx := &CommandContext{
            NATSClient:    r.natsClient,
            LoopTracker:   r.loopTracker,
            Logger:        r.logger,
            HasPermission: r.hasPermission,
        }
        loopID := r.loopTracker.GetActiveLoop(msg.UserID, msg.ChannelID)
        return executor.Execute(ctx, cmdCtx, msg, args, loopID)
    }
}

// CommandRegistry exposes the registry for applications to register commands
// (per-component registration approach)
func (r *Router) CommandRegistry() *CommandRegistry {
    return r.registry
}

func (r *Router) handleCommand(ctx context.Context, msg UserMessage) error {
    // Find matching command
    name, reg, args, found := r.registry.Match(msg.Content)
    if !found {
        return r.sendError(ctx, msg, "Unknown command. Type /help for available commands.")
    }
    
    // Check permission
    if reg.Config.Permission != "" {
        if err := r.checkPermission(msg.UserID, reg.Config.Permission); err != nil {
            return r.sendPermissionError(ctx, msg, reg.Config.Permission)
        }
    }
    
    // Execute handler
    resp, err := reg.Handler(ctx, msg, args)
    if err != nil {
        return r.sendError(ctx, msg, err.Error())
    }
    
    return r.sendResponse(ctx, msg, resp)
}

func (r *Router) registerBuiltinCommands() {
    // User commands
    r.registry.Register("cancel", CommandConfig{
        Pattern:    `^/cancel\s*(\S+)?$`,
        Permission: "cancel_own",
        Scope:      "user",
        Help:       "/cancel [loop_id] - Cancel current or specified loop",
    }, r.handleCancel)
    
    r.registry.Register("status", CommandConfig{
        Pattern:    `^/status\s*(\S+)?$`,
        Permission: "view",
        Scope:      "user",
        Help:       "/status [loop_id] - Show loop status",
    }, r.handleStatus)
    
    r.registry.Register("loops", CommandConfig{
        Pattern:    `^/loops$`,
        Permission: "view",
        Scope:      "user",
        Help:       "/loops - List your active loops",
    }, r.handleLoops)
    
    // System commands
    r.registry.Register("ps", CommandConfig{
        Pattern:    `^/ps$`,
        Permission: "view_system",
        Scope:      "system",
        Help:       "/ps - List ALL active loops",
    }, r.handlePs)
    
    r.registry.Register("top", CommandConfig{
        Pattern:    `^/top$`,
        Permission: "view_system",
        Scope:      "system",
        Help:       "/top - Live activity feed",
    }, r.handleTop)
    
    r.registry.Register("system", CommandConfig{
        Pattern:    `^/system$`,
        Permission: "view_system",
        Scope:      "system",
        Help:       "/system - Component health and stats",
    }, r.handleSystem)
    
    r.registry.Register("kill", CommandConfig{
        Pattern:    `^/kill\s+(\S+)$`,
        Permission: "cancel_any",
        Scope:      "system",
        Help:       "/kill <loop_id> - Force cancel any loop",
    }, r.handleKill)
    
    r.registry.Register("help", CommandConfig{
        Pattern:    `^/help$`,
        Permission: "",
        Scope:      "user",
        Help:       "/help - Show available commands",
    }, r.handleHelp)
    
    // ... other built-in commands
}
```

### Application Registration (Semspec Example)

This example shows both global registration (preferred) and per-component registration approaches.

**Approach 1: Global Registration (Preferred)**

```go
// semspec/commands/propose.go
package commands

import (
    "context"
    "github.com/c360/semstreams/agentic"
    "agenticdispatch "github.com/c360/semstreams/processor/agentic-dispatch""
)

func init() {
    router.RegisterCommand("propose", &ProposeCommand{})
}

type ProposeCommand struct {
    entityStore EntityStore // Set via dependency injection
}

func (c *ProposeCommand) Config() router.CommandConfig {
    return router.CommandConfig{
        Pattern:    `^/propose\s+(.+)$`,
        Permission: "submit_task",
        Scope:      "user",
        Category:   "semspec",
        Help:       "/propose <description> - Create new proposal",
    }
}

func (c *ProposeCommand) Execute(ctx context.Context, cmdCtx *router.CommandContext, msg agentic.UserMessage, args []string, loopID string) (agentic.UserResponse, error) {
    if len(args) == 0 {
        return agentic.UserResponse{}, fmt.Errorf("usage: /propose <description>")
    }

    description := args[0]

    // Create proposal entity
    proposal := &entity.Proposal{
        ID:          "proposal:" + uuid.New().String()[:8],
        Title:       extractTitle(description),
        Description: description,
        Status:      "exploring",
        CreatedBy:   msg.UserID,
        CreatedAt:   time.Now(),
    }

    if err := c.entityStore.Put(ctx, proposal.ID, proposal); err != nil {
        return agentic.UserResponse{}, err
    }

    // Start exploration loop using CommandContext
    newLoopID := "loop_" + proposal.ID[9:]
    task := agentic.TaskMessage{
        LoopID: newLoopID,
        TaskID: uuid.New().String(),
        Role:   "planner",
        Prompt: fmt.Sprintf("Explore this proposal and identify questions:\n\n%s", description),
        Metadata: map[string]string{
            "proposal_id": proposal.ID,
        },
    }

    data, _ := json.Marshal(task)
    if err := cmdCtx.NATSClient.Publish(ctx, "agent.task."+task.TaskID, data); err != nil {
        return agentic.UserResponse{}, err
    }

    return agentic.UserResponse{
        Type:    "status",
        Content: fmt.Sprintf("Created proposal %s\nStarting exploration...", proposal.ID),
    }, nil
}
```

**Approach 2: Per-Component Registration**

For component-specific commands that need access to component state:

```go
// semspec/commands.go

func (s *Semspec) RegisterCommands(registry *router.CommandRegistry) error {
    // Register component-specific command handlers
    if err := registry.Register("tasks", router.CommandConfig{
        Pattern:    `^/tasks(?:\s+(\w+))?$`,
        Permission: "view",
        Scope:      "user",
        Category:   "semspec",
        Help:       "/tasks [status] - List tasks",
    }, s.handleTasks); err != nil {
        return err
    }

    if err := registry.Register("constitution", router.CommandConfig{
        Pattern:    `^/constitution$`,
        Permission: "view",
        Scope:      "user",
        Category:   "semspec",
        Help:       "/constitution - Show project constitution",
    }, s.handleConstitution); err != nil {
        return err
    }

    return nil
}

// Command handlers have access to semspec state
func (s *Semspec) handleTasks(ctx context.Context, msg router.UserMessage, args []string) (router.UserResponse, error) {
    statusFilter := ""
    if len(args) > 0 {
        statusFilter = args[0]
    }

    tasks, err := s.entityStore.Query(ctx, "task:*")
    if err != nil {
        return router.UserResponse{}, err
    }

    var lines []string
    for _, t := range tasks {
        task := t.(*entity.Task)
        if statusFilter != "" && task.Status != statusFilter {
            continue
        }
        lines = append(lines, fmt.Sprintf("%-12s %-10s %s",
            task.ID[5:13], task.Status, truncate(task.Title, 40)))
    }

    if len(lines) == 0 {
        return router.UserResponse{
            Type:    "text",
            Content: "No tasks found",
        }, nil
    }

    header := "TASK         STATUS     TITLE"
    return router.UserResponse{
        Type:    "text",
        Content: header + "\n" + strings.Join(lines, "\n"),
    }, nil
}

func (s *Semspec) handleConstitution(ctx context.Context, msg router.UserMessage, args []string) (router.UserResponse, error) {
    constitution, err := s.entityStore.Get(ctx, "constitution:"+s.config.Project)
    if err != nil {
        return router.UserResponse{}, fmt.Errorf("no constitution found for project")
    }

    c := constitution.(*entity.Constitution)

    var sections []string
    if len(c.CodeQuality) > 0 {
        sections = append(sections, "CODE QUALITY\n  • "+strings.Join(c.CodeQuality, "\n  • "))
    }
    if len(c.Testing) > 0 {
        sections = append(sections, "TESTING\n  • "+strings.Join(c.Testing, "\n  • "))
    }
    if len(c.Security) > 0 {
        sections = append(sections, "SECURITY\n  • "+strings.Join(c.Security, "\n  • "))
    }
    if len(c.Architecture) > 0 {
        sections = append(sections, "ARCHITECTURE\n  • "+strings.Join(c.Architecture, "\n  • "))
    }

    return router.UserResponse{
        Type:    "text",
        Content: strings.Join(sections, "\n\n"),
    }, nil
}
```

### Startup Integration

**With global registration** (commands auto-loaded via init()):

```go
// semspec/main.go

func main() {
    // ... setup ...

    // Create router - automatically loads global commands
    router := router.NewRouter(routerConfig, natsClient)

    // Create Semspec (global commands already registered)
    semspec := semspec.New(config, natsClient, entityStore, queryEngine)

    // Optional: register component-specific commands
    if err := semspec.RegisterCommands(router.CommandRegistry()); err != nil {
        log.Fatal(err)
    }

    // Start components
    // ...
}
```

**With per-component registration only**:

```go
// semspec/main.go

func main() {
    // ... setup ...

    // Create router
    router := router.NewRouter(routerConfig, natsClient)

    // Create Semspec
    semspec := semspec.New(config, natsClient, entityStore, queryEngine)

    // Register Semspec commands with router
    if err := semspec.RegisterCommands(router.CommandRegistry()); err != nil {
        log.Fatal(err)
    }

    // Start components
    // ...
}
```

### Help Output with Registered Commands

```
semspec> /help

BUILT-IN COMMANDS
  /cancel [id]        Cancel current or specified loop
  /status [id]        Show loop status
  /loops              List your active loops
  /approve [id]       Approve pending result
  /reject [id] [why]  Reject with reason
  /history [n]        Show recent loop history

SYSTEM COMMANDS
  /ps                 List ALL active loops
  /top                Live activity feed
  /system             Component health and stats
  /kill <id>          Force cancel any loop

SEMSPEC COMMANDS
  /propose <desc>     Create new proposal
  /spec [id]          Show or create specification
  /review [id]        Review pending result
  /tasks [status]     List tasks
  /constitution       Show project constitution
  /explore <topic>    Free exploration mode
  /plan [proposal]    Create plan from proposal
  /graph <query>      Query knowledge graph
```

### Command Categories

Commands can be grouped by source for help display:

```go
type CommandConfig struct {
    Pattern     string `json:"pattern"`
    Permission  string `json:"permission"`
    Scope       string `json:"scope"`      // user | system
    Category    string `json:"category"`   // built-in | semspec | custom
    Help        string `json:"help"`

    compiledPattern *regexp.Regexp
}

func (r *Router) handleHelp(ctx context.Context, msg UserMessage, args []string) (UserResponse, error) {
    commands := r.registry.All()

    // Group by category
    categories := make(map[string][]string)
    for name, config := range commands {
        // Skip if user doesn't have permission
        if config.Permission != "" && !r.hasPermission(msg.UserID, config.Permission) {
            continue
        }

        category := config.Category
        if category == "" {
            category = "built-in"
        }

        categories[category] = append(categories[category],
            fmt.Sprintf("  %-18s %s", "/"+name, config.Help))
    }

    var sections []string
    for _, cat := range []string{"built-in", "system", "semspec"} {
        if cmds, ok := categories[cat]; ok {
            sort.Strings(cmds)
            sections = append(sections,
                strings.ToUpper(cat)+" COMMANDS\n"+strings.Join(cmds, "\n"))
        }
    }

    return UserResponse{
        Type:    "text",
        Content: strings.Join(sections, "\n\n"),
    }, nil
}
```

### Command Registration Summary

**When to use global registration**:

- Reusable commands that work across projects
- Commands that don't need component-specific state
- Commands from external packages or libraries
- Following the SemStreams pattern (matches tool and component registration)

**When to use per-component registration**:

- Commands that need access to component instance state
- Commands tightly coupled to a specific component lifecycle
- Commands that require dependency injection at component creation time

**Key differences**:

| Aspect | Global (init) | Per-Component |
|--------|--------------|---------------|
| Registration timing | Package init | Component creation |
| State access | Via CommandContext | Direct component access |
| Reusability | High | Component-specific |
| Pattern | `CommandExecutor` interface | `CommandHandler` function |
| Dependencies | Via CommandContext | Via component closure |

Both approaches can coexist in the same application. Global commands are loaded first, then per-component
commands are added.

| Subject Pattern | Publisher | Subscriber | Purpose |
|-----------------|-----------|------------|---------|
| `user.message.{type}.{id}` | input/* | router | User input |
| `user.signal.{loop_id}` | input/*, router | router | Control signals |
| `user.response.{type}.{id}` | router | input/* | Responses to user |
| `agent.task.*` | router | agentic-loop | New tasks |
| `agent.signal.*` | router | agentic-loop | Signals to loops |
| `agent.complete.*` | agentic-loop | router | Loop completions |
| `agent.status.*` | agentic-loop | router | Status updates (for /top) |

---

## Part 7: Subject Summary

| Subject Pattern | Publisher | Subscriber | Purpose |
|-----------------|-----------|------------|---------|
| `user.message.{type}.{id}` | input/* | router | User input |
| `user.signal.{loop_id}` | input/*, router | router | Control signals |
| `user.response.{type}.{id}` | router | input/* | Responses to user |
| `agent.task.*` | router | agentic-loop | New tasks |
| `agent.signal.*` | router | agentic-loop | Signals to loops |
| `agent.complete.*` | agentic-loop | router | Loop completions |
| `agent.status.*` | agentic-loop | router | Status updates (for /top) |

---

## Part 8: HTTP Endpoints

Components expose HTTP endpoints via the Service Manager for programmatic access by UIs and external systems.

> **NOTE**: The interface below aligns with the EXISTING `HTTPHandler` pattern in `service/openapi.go`.
> This is NOT a new port type - HTTP is coordinated centrally by ServiceManager.

### Interface (EXISTING in `service/openapi.go`)

```go
// service/openapi.go - EXISTING interface
type HTTPHandler interface {
    // RegisterHTTPHandlers adds component's HTTP handlers
    // prefix will be "/api/{component-name}"
    RegisterHTTPHandlers(prefix string, mux *http.ServeMux)
    
    // OpenAPISpec returns the OpenAPI specification for the component
    OpenAPISpec() *OpenAPISpec
}
```

Components that want HTTP endpoints implement this interface and register with ServiceManager.

### Service Manager Core Endpoints (EXISTING)

```
GET  /health                  # Overall system health (EXISTING)
GET  /readyz                  # Readiness probe (EXISTING)
GET  /metrics                 # Prometheus-format metrics (EXISTING)
GET  /components/health       # Aggregated component health (EXISTING)
GET  /components/list         # List registered components (EXISTING)
GET  /components/status/{name} # Component status (EXISTING)
```

### NEW Endpoints to Add

```
GET  /api/components          # List registered components + status (NEW)
GET  /api/streams             # NATS stream stats (NEW)
GET  /stream/activity         # SSE: real-time activity stream (NEW)
WS   /ws/activity             # WebSocket: real-time activity stream (NEW)
```

### Router Endpoints (`/api/router/...`)

```
GET  /api/router/loops
     List active loops with filtering
     
     Query params:
       ?owner=coby         # filter by owner
       ?source=cli         # filter by source  
       ?state=executing    # filter by state
       ?limit=50           # pagination
     
     Response:
     {
       "loops": [
         {
           "loop_id": "abc123",
           "owner": "coby",
           "source": "cli",
           "state": "executing",
           "iterations": 3,
           "max_iterations": 20,
           "started_at": "2025-01-28T10:30:00Z",
           "prompt_preview": "add health endpoint..."
         }
       ],
       "total": 4,
       "filters_applied": {"owner": "coby"}
     }

GET  /api/router/loops/:id
     Single loop details
     
     Response:
     {
       "loop_id": "abc123",
       "owner": "coby",
       "source": "cli",
       "channel_type": "cli",
       "channel_id": "terminal-1",
       "state": "executing",
       "role": "implementer",
       "model": "qwen2.5-coder:32b",
       "iterations": 3,
       "max_iterations": 20,
       "pending_tools": ["call_xyz"],
       "started_at": "2025-01-28T10:30:00Z",
       "prompt": "add health endpoint to gateway"
     }

POST /api/router/loops/:id/signal
     Send signal to loop
     
     Body: {"type": "cancel", "payload": null}
     Response: {"acknowledged": true}

GET  /api/router/commands
     List available commands with permissions
     
     Response:
     {
       "commands": [
         {"name": "cancel", "pattern": "/cancel [id]", "permission": "cancel_own", "scope": "user"},
         {"name": "ps", "pattern": "/ps", "permission": "view_system", "scope": "system"}
       ]
     }

GET  /api/router/stats
     Router statistics
     
     Response:
     {
       "messages_routed": 1247,
       "signals_processed": 34,
       "active_loops": 4,
       "loops_completed_today": 23,
       "average_loop_duration_ms": 45000
     }
```

### Agentic-Loop Endpoints (`/api/agentic-loop/...`)

```
GET  /api/agentic-loop/loops
     All loops from loop processor's perspective
     
GET  /api/agentic-loop/loops/:id
     Detailed loop entity from KV
     
GET  /api/agentic-loop/loops/:id/trajectory
     Full trajectory for a loop
     
     Query params:
       ?format=json|jsonl   # output format
       ?include=all|summary # detail level
     
     Response (summary):
     {
       "loop_id": "abc123",
       "steps": 12,
       "tool_calls": 8,
       "model_calls": 4,
       "tokens_in": 15420,
       "tokens_out": 8930,
       "duration_ms": 45000
     }

GET  /api/agentic-loop/stats
     Loop processor statistics
     
     Response:
     {
       "active_loops": 4,
       "pending_model_requests": 1,
       "pending_tool_calls": 2,
       "completed_today": 23,
       "failed_today": 2,
       "cancelled_today": 1
     }
```

### Agentic-Model Endpoints (`/api/agentic-model/...`)

```
GET  /api/agentic-model/status
     Model connectivity status
     
     Response:
     {
       "endpoints": [
         {
           "name": "ollama-local",
           "url": "http://localhost:11434/v1",
           "status": "connected",
           "last_request": "2025-01-28T10:30:00Z",
           "models_available": ["qwen2.5-coder:32b", "deepseek-coder-v2:16b"]
         }
       ]
     }

GET  /api/agentic-model/stats
     Model call statistics
     
     Response:
     {
       "requests_today": 156,
       "tokens_in_today": 234500,
       "tokens_out_today": 89200,
       "average_latency_ms": 1200,
       "errors_today": 3,
       "by_model": {
         "qwen2.5-coder:32b": {"requests": 120, "tokens_in": 200000}
       }
     }

GET  /api/agentic-model/queue
     Current request queue
     
     Response:
     {
       "queued": 1,
       "processing": 1,
       "requests": [
         {"request_id": "req_123", "loop_id": "abc123", "queued_at": "..."}
       ]
     }
```

### Agentic-Tools Endpoints (`/api/agentic-tools/...`)

```
GET  /api/agentic-tools/executors
     Registered tool executors
     
     Response:
     {
       "executors": [
         {"name": "file_read", "enabled": true, "calls_today": 45},
         {"name": "file_write", "enabled": true, "calls_today": 12},
         {"name": "git_commit", "enabled": true, "calls_today": 8},
         {"name": "graph_query", "enabled": true, "calls_today": 67}
       ]
     }

GET  /api/agentic-tools/pending
     Currently executing tools
     
     Response:
     {
       "pending": [
         {
           "call_id": "call_xyz",
           "tool": "file_read",
           "loop_id": "abc123",
           "started_at": "2025-01-28T10:30:00Z",
           "timeout_at": "2025-01-28T10:30:30Z"
         }
       ]
     }

GET  /api/agentic-tools/stats
     Tool execution statistics
     
     Response:
     {
       "calls_today": 234,
       "errors_today": 5,
       "average_duration_ms": 150,
       "by_tool": {
         "file_read": {"calls": 89, "avg_ms": 50},
         "graph_query": {"calls": 67, "avg_ms": 200}
       }
     }
```

### Activity Stream Endpoints

```
GET  /stream/activity
     Server-Sent Events stream of all activity
     
     Query params:
       ?filter=loop:abc123    # filter to specific loop
       ?types=tool,model      # filter event types
     
     Events:
     data: {"type": "loop_started", "loop_id": "abc123", "timestamp": "..."}
     data: {"type": "model_request", "loop_id": "abc123", "model": "qwen2.5-coder:32b"}
     data: {"type": "tool_call", "loop_id": "abc123", "tool": "file_read"}
     data: {"type": "tool_result", "loop_id": "abc123", "tool": "file_read", "duration_ms": 45}
     data: {"type": "loop_complete", "loop_id": "abc123", "outcome": "success"}

WS   /ws/activity
     WebSocket alternative (same events as SSE)
     
     Client can send filter updates:
     {"action": "filter", "loop_id": "abc123"}
     {"action": "unfilter"}
```

### Endpoint Summary Table

| Endpoint | Component | Purpose |
|----------|-----------|---------|
| `GET /api/health` | ServiceManager | Overall health |
| `GET /api/components` | ServiceManager | Component status |
| `GET /api/metrics` | ServiceManager | Prometheus metrics |
| `GET /stream/activity` | ServiceManager | SSE activity stream |
| `WS /ws/activity` | ServiceManager | WebSocket activity |
| `GET /api/router/loops` | Router | List/filter loops |
| `GET /api/router/loops/:id` | Router | Loop details |
| `POST /api/router/loops/:id/signal` | Router | Send signal |
| `GET /api/router/stats` | Router | Routing stats |
| `GET /api/agentic-loop/loops/:id/trajectory` | AgenticLoop | Full trajectory |
| `GET /api/agentic-loop/stats` | AgenticLoop | Loop stats |
| `GET /api/agentic-model/status` | AgenticModel | Model connectivity |
| `GET /api/agentic-model/stats` | AgenticModel | Token usage |
| `GET /api/agentic-tools/executors` | AgenticTools | Registered tools |
| `GET /api/agentic-tools/stats` | AgenticTools | Tool stats |

### Implementation Example

```go
// processor/agentic-dispatch/http.go
func (r *Router) RegisterRoutes(mux *http.ServeMux, prefix string) {
    mux.HandleFunc(prefix+"/loops", r.handleGetLoops)
    mux.HandleFunc(prefix+"/loops/", r.handleLoopByID)
    mux.HandleFunc(prefix+"/commands", r.handleGetCommands)
    mux.HandleFunc(prefix+"/stats", r.handleGetStats)
}

func (r *Router) handleGetLoops(w http.ResponseWriter, req *http.Request) {
    if req.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    // Parse filters
    owner := req.URL.Query().Get("owner")
    source := req.URL.Query().Get("source")
    state := req.URL.Query().Get("state")
    
    loops := r.loopTracker.GetFiltered(owner, source, state)
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(LoopsResponse{
        Loops: loops,
        Total: len(loops),
    })
}
```

---

## Part 9: Build Order

> **Legend**: Items marked **(NEW)** do not exist. Items marked **(EXTEND)** modify existing code.

### Phase 1 (MVP)

| Item | Type | Description |
|------|------|-------------|
| `processor/input/cli/` | NEW | Basic stdin handling, publishes `user.message.cli.*` |
| `processor/agentic-dispatch/` | NEW | Command parsing, basic permissions, routing |
| `UserMessage`, `UserSignal`, `UserResponse` types | NEW | Add to `agentic/` or new `interaction/` package |
| `user.message.*`, `user.signal.*`, `user.response.*` | NEW | New NATS subject patterns |
| Signal handling in agentic-loop | EXTEND | Add `agent.signal.*` subscription, handlers for cancel/approve/reject |
| New loop states | EXTEND | Add `paused`, `cancelled`, `awaiting_approval` to `agentic/state.go` |
| `CommandRegistry` | NEW | Command registration infrastructure |
| `LoopTracker` | NEW | Track active loops per user/channel |

### Phase 2 (System Visibility)

| Item | Type | Description |
|------|------|-------------|
| System commands | NEW | `/ps`, `/top`, `/system` in router |
| Router HTTP endpoints | EXTEND | Implement `HTTPHandler` interface on router |
| agentic-loop HTTP endpoints | EXTEND | Implement `HTTPHandler` interface |
| Activity stream (SSE) | NEW | `/stream/activity` endpoint |
| Loop tracking persistence | EXTEND | Survive restarts via KV |

### Phase 3 (Enhanced)

| Item | Type | Description |
|------|------|-------------|
| Pause/resume support | EXTEND | Signal handlers + state transitions |
| Rate limiting | NEW | `RateLimiter` in router |
| agentic-model HTTP endpoints | EXTEND | Implement `HTTPHandler` interface |
| agentic-tools HTTP endpoints | EXTEND | Implement `HTTPHandler` interface |
| WebSocket activity stream | NEW | `/ws/activity` endpoint |

### Phase 4 (Multi-channel) - Optional

| Item | Type | Description |
|------|------|-------------|
| `processor/input/slack/` | NEW | Slack Bolt integration |
| `processor/input/discord/` | NEW | Discord.go integration |
| `processor/input/web/` | NEW | HTTP/WebSocket API for web clients |
