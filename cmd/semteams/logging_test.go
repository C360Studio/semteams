package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

type publishedLogMessage struct {
	subject     string
	data        []byte
	hasDeadline bool
	deadlineIn  time.Duration
}

type fakeLogPublisher struct {
	ch  chan publishedLogMessage
	err error
}

func newFakeLogPublisher(buf int) *fakeLogPublisher {
	return &fakeLogPublisher{ch: make(chan publishedLogMessage, buf)}
}

func (f *fakeLogPublisher) PublishToStream(ctx context.Context, subject string, data []byte) error {
	deadline, hasDeadline := ctx.Deadline()
	var remaining time.Duration
	if hasDeadline {
		remaining = time.Until(deadline)
	}
	copied := append([]byte(nil), data...)
	f.ch <- publishedLogMessage{
		subject:     subject,
		data:        copied,
		hasDeadline: hasDeadline,
		deadlineIn:  remaining,
	}
	return f.err
}

func waitPublishedLog(t *testing.T, ch <-chan publishedLogMessage) publishedLogMessage {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async NATS log publish")
		return publishedLogMessage{}
	}
}

func assertNoPublishedLog(t *testing.T, ch <-chan publishedLogMessage) {
	t.Helper()
	for range 32 {
		runtime.Gosched()
	}
	select {
	case msg := <-ch:
		t.Fatalf("expected log record to be filtered out, but published to %q: %s", msg.subject, string(msg.data))
	default:
	}
}

type resolvedToken struct {
	val string
}

func (rt resolvedToken) LogValue() slog.Value {
	return slog.StringValue("resolved:" + rt.val)
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		input string
		want  slog.Level
	}{
		{input: "debug", want: slog.LevelDebug},
		{input: "DEBUG", want: slog.LevelDebug},
		{input: "info", want: slog.LevelInfo},
		{input: "Info", want: slog.LevelInfo},
		{input: "warn", want: slog.LevelWarn},
		{input: "WARN", want: slog.LevelWarn},
		{input: "error", want: slog.LevelError},
		{input: "ErRoR", want: slog.LevelError},
		{input: "", want: slog.LevelInfo},
		{input: "trace", want: slog.LevelInfo},
	}

	for _, tc := range cases {
		if got := parseLogLevel(tc.input); got != tc.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestNewLoggerStdoutFormattingAndMetadata(t *testing.T) {
	t.Run("json_default_and_top_level_attrs_without_debug_source", func(t *testing.T) {
		var buf bytes.Buffer
		pub := newFakeLogPublisher(4)
		logger := newLogger(&buf, "info", "unknown-format", pub, nil)

		logger.Debug("suppressed debug line")
		assertNoPublishedLog(t, pub.ch)
		if buf.Len() != 0 {
			t.Fatalf("expected debug line to be suppressed at info level, got %q", buf.String())
		}

		logger.Info("boot complete", "port", 8080)
		msg := waitPublishedLog(t, pub.ch)
		if msg.subject != "logs.INFO."+appName {
			t.Fatalf("subject = %q, want %q", msg.subject, "logs.INFO."+appName)
		}

		var stdoutJSON map[string]any
		if err := json.Unmarshal(buf.Bytes(), &stdoutJSON); err != nil {
			t.Fatalf("stdout is not valid JSON: %v (%q)", err, buf.String())
		}
		if stdoutJSON["msg"] != "boot complete" || stdoutJSON["level"] != "INFO" {
			t.Fatalf("unexpected stdout msg/level: %v", stdoutJSON)
		}
		if stdoutJSON["service"] != appName || stdoutJSON["version"] != version {
			t.Fatalf("missing service/version attrs: %v", stdoutJSON)
		}
		if gotPID, ok := stdoutJSON["pid"].(float64); !ok || int(gotPID) != os.Getpid() {
			t.Fatalf("stdout pid = %v, want %d", stdoutJSON["pid"], os.Getpid())
		}
		if _, hasSource := stdoutJSON["source"]; hasSource {
			t.Fatalf("expected AddSource=false at info level, got source=%v", stdoutJSON["source"])
		}
	})

	t.Run("debug_enables_add_source_and_text_format_renders_key_values", func(t *testing.T) {
		var jsonBuf bytes.Buffer
		pub := newFakeLogPublisher(4)
		debugLogger := newLogger(&jsonBuf, "DEBUG", "json", pub, nil)
		debugLogger.Debug("debug probe")
		_ = waitPublishedLog(t, pub.ch)

		var debugEntry map[string]any
		if err := json.Unmarshal(jsonBuf.Bytes(), &debugEntry); err != nil {
			t.Fatalf("unmarshal debug JSON: %v", err)
		}
		if _, hasSource := debugEntry["source"]; !hasSource {
			t.Fatalf("expected AddSource=true when level is debug, got %v", debugEntry)
		}

		var textBuf bytes.Buffer
		textLogger := newLogger(&textBuf, "info", "TEXT", pub, nil)
		textLogger.Warn("text mode warning", "component", "rule-processor")
		natsMsg := waitPublishedLog(t, pub.ch)
		if natsMsg.subject != "logs.WARN.rule-processor" {
			t.Fatalf("subject = %q, want logs.WARN.rule-processor", natsMsg.subject)
		}
		out := textBuf.String()
		for _, token := range []string{"level=WARN", `msg="text mode warning"`, "service=" + appName, "component=rule-processor"} {
			if !strings.Contains(out, token) {
				t.Fatalf("text output %q missing %q", out, token)
			}
		}
	})
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestNATSLogHandlerSourcePrecedenceAndPayload(t *testing.T) {
	pub := newFakeLogPublisher(8)
	base := newNATSLogHandler(pub, slog.LevelDebug, nil)

	// 1. Default fallback to "system" and omit "fields" when no extra attributes exist.
	slog.New(base).Info("bare system event")
	bareMsg := waitPublishedLog(t, pub.ch)
	if bareMsg.subject != "logs.INFO.system" {
		t.Fatalf("bare subject = %q, want logs.INFO.system", bareMsg.subject)
	}
	var bareEntry map[string]any
	if err := json.Unmarshal(bareMsg.data, &bareEntry); err != nil {
		t.Fatalf("unmarshal bare entry: %v", err)
	}
	if bareEntry["source"] != "system" || bareEntry["level"] != "INFO" || bareEntry["message"] != "bare system event" {
		t.Fatalf("unexpected bare entry: %v", bareEntry)
	}
	if ts, ok := bareEntry["timestamp"].(string); !ok {
		t.Fatalf("missing timestamp string: %v", bareEntry)
	} else if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Fatalf("timestamp %q is not RFC3339Nano: %v", ts, err)
	}
	if _, hasFields := bareEntry["fields"]; hasFields {
		t.Fatalf("expected empty fields map to be omitted, got %v", bareEntry["fields"])
	}

	// 2. Priority: source > component > service, and record attrs override WithAttrs attrs.
	withService := slog.New(base).With("service", "svc-a")
	withService.Info("service level")
	if got := waitPublishedLog(t, pub.ch).subject; got != "logs.INFO.svc-a" {
		t.Fatalf("service subject = %q, want logs.INFO.svc-a", got)
	}

	withComponent := withService.With("component", "comp-b")
	withComponent.Warn("component overrides service")
	if got := waitPublishedLog(t, pub.ch).subject; got != "logs.WARN.comp-b" {
		t.Fatalf("component subject = %q, want logs.WARN.comp-b", got)
	}

	withSource := withComponent.With("source", "src-c")
	withSource.Error("record source overrides handler source",
		"source", "src-record",
		"attempt", 3,
		"token", resolvedToken{val: "ok"},
	)
	overrideMsg := waitPublishedLog(t, pub.ch)
	if overrideMsg.subject != "logs.ERROR.src-record" {
		t.Fatalf("override subject = %q, want logs.ERROR.src-record", overrideMsg.subject)
	}

	var entry map[string]any
	if err := json.Unmarshal(overrideMsg.data, &entry); err != nil {
		t.Fatalf("unmarshal override entry: %v", err)
	}
	fields, ok := entry["fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected non-empty fields map, got %v", entry)
	}
	if fields["attempt"] != float64(3) || fields["token"] != "resolved:ok" {
		t.Fatalf("unexpected fields content: %v", fields)
	}
	for _, omitted := range []string{"source", "component", "service"} {
		if _, exists := fields[omitted]; exists {
			t.Fatalf("fields must omit top-level routing key %q, got %v", omitted, fields)
		}
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestGetExcludeSourcesAndPrefixFiltering(t *testing.T) {
	defaultExcluded := []string{"flow-service.websocket"}
	disabledCfg := &config.Config{
		Services: types.ServiceConfigs{
			"log-forwarder": {Enabled: false, Config: json.RawMessage(`{"exclude_sources":["custom.src"]}`)},
		},
	}
	emptyListCfg := &config.Config{
		Services: types.ServiceConfigs{
			"log-forwarder": {Enabled: true, Config: json.RawMessage(`{"exclude_sources":[]}`)},
		},
	}
	badJSONCfg := &config.Config{
		Services: types.ServiceConfigs{
			"log-forwarder": {Enabled: true, Config: json.RawMessage(`{invalid`)},
		},
	}
	for name, cfg := range map[string]*config.Config{
		"nil":        nil,
		"empty_svcs": {},
		"disabled":   disabledCfg,
		"empty_list": emptyListCfg,
		"bad_json":   badJSONCfg,
	} {
		got := getExcludeSources(cfg)
		if len(got) != 1 || got[0] != defaultExcluded[0] {
			t.Fatalf("%s: getExcludeSources = %v, want %v", name, got, defaultExcluded)
		}
	}

	enabledCfg := &config.Config{
		Services: types.ServiceConfigs{
			"log-forwarder": {Enabled: true, Config: json.RawMessage(`{"exclude_sources":["flow-service.websocket","noisy.worker"]}`)},
		},
	}
	gotCustom := getExcludeSources(enabledCfg)
	if len(gotCustom) != 2 || gotCustom[0] != "flow-service.websocket" || gotCustom[1] != "noisy.worker" {
		t.Fatalf("enabled custom getExcludeSources = %v", gotCustom)
	}

	// Verify defensive slice copy + exact/dot-prefix matching vs parent/sibling non-match.
	callerSlice := []string{"flow-service.websocket"}
	pub := newFakeLogPublisher(8)
	h := newNATSLogHandler(pub, slog.LevelInfo, callerSlice)
	callerSlice[0] = "mutated.caller.slice"

	logger := slog.New(h)
	logger.Info("exact match dropped", "source", "flow-service.websocket")
	assertNoPublishedLog(t, pub.ch)

	logger.Info("dot-prefix child dropped", "source", "flow-service.websocket.health")
	assertNoPublishedLog(t, pub.ch)

	logger.Info("parent allowed", "source", "flow-service")
	if got := waitPublishedLog(t, pub.ch).subject; got != "logs.INFO.flow-service" {
		t.Fatalf("parent subject = %q, want logs.INFO.flow-service", got)
	}

	logger.Info("sibling without dot boundary allowed", "source", "flow-service.websocket2")
	if got := waitPublishedLog(t, pub.ch).subject; got != "logs.INFO.flow-service.websocket2" {
		t.Fatalf("sibling subject = %q, want logs.INFO.flow-service.websocket2", got)
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestNATSLogHandlerContractAndConcurrency(t *testing.T) {
	t.Run("nil_publisher_panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected newNATSLogHandler(nil, ...) to panic")
			}
		}()
		_ = newNATSLogHandler(nil, slog.LevelInfo, nil)
	})

	t.Run("handler_identity_deadline_and_error_drop", func(t *testing.T) {
		pub := newFakeLogPublisher(4)
		pub.err = errors.New("simulated jetstream publish failure")
		h := newNATSLogHandler(pub, slog.LevelInfo, nil)

		if h.WithAttrs(nil) != h {
			t.Fatal("WithAttrs(nil) must return receiver unchanged")
		}
		if h.WithGroup("") != h {
			t.Fatal("WithGroup(\"\") must return receiver unchanged")
		}
		grouped := h.WithGroup("http")
		if grouped == h {
			t.Fatal("WithGroup(\"http\") must return a new handler")
		}

		// Also exercise setupLogger wrapper at error level so stdout stays quiet on info.
		logger := setupLogger("error", "json", pub, nil)
		logger.Info("filtered below error")
		assertNoPublishedLog(t, pub.ch)

		slog.New(grouped).Error("bounded publish check", "source", "api")
		msg := waitPublishedLog(t, pub.ch)
		if msg.subject != "logs.ERROR.api" {
			t.Fatalf("subject = %q, want logs.ERROR.api", msg.subject)
		}
		if !msg.hasDeadline || msg.deadlineIn <= 0 || msg.deadlineIn > 5*time.Second {
			t.Fatalf("expected bounded publish deadline in (0, 5s], got hasDeadline=%v remaining=%v",
				msg.hasDeadline, msg.deadlineIn)
		}
	})

	t.Run("concurrent_logging_race_safe", func(t *testing.T) {
		const workers = 8
		const perWorker = 5
		pub := newFakeLogPublisher(workers * perWorker)
		var stdout bytes.Buffer
		var mu sync.Mutex
		syncWriter := writerFunc(func(p []byte) (int, error) {
			mu.Lock()
			defer mu.Unlock()
			return stdout.Write(p)
		})

		baseLogger := newLogger(syncWriter, "info", "json", pub, nil)
		var wg sync.WaitGroup
		for w := range workers {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				child := baseLogger.With("component", "worker").WithGroup("batch")
				for i := range perWorker {
					child.Info("concurrent log", "worker_id", id, "seq", i)
				}
			}(w)
		}
		wg.Wait()

		for range workers * perWorker {
			msg := waitPublishedLog(t, pub.ch)
			if msg.subject != "logs.INFO.worker" {
				t.Fatalf("unexpected concurrent subject %q", msg.subject)
			}
		}
	})
}

type writerFunc func([]byte) (int, error)

func (wf writerFunc) Write(p []byte) (int, error) {
	return wf(p)
}
