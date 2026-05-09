package verification

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/payloadregistry"
)

func TestRuntimeKind_IsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    RuntimeKind
		want bool
	}{
		{RuntimeInProcessUnit, true},
		{RuntimeProcessLocalTestcontainer, true},
		{RuntimeExternalSidecar, true},
		{RuntimeBrowserFlow, true},
		{RuntimeStaticAnalysis, true},
		{"", false},
		{"unknown", false},
		{"INPROCESS_UNIT", false}, // case-sensitive
		{"in-process-unit ", false},
		{"in_process_unit", false}, // underscore typo, common LLM emission failure mode
	}
	for _, c := range cases {
		t.Run(string(c.k), func(t *testing.T) {
			if got := c.k.IsValid(); got != c.want {
				t.Errorf("%q.IsValid() = %v, want %v", c.k, got, c.want)
			}
		})
	}
}

func TestRuntimeKind_RequiresTestHarness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    RuntimeKind
		want bool
	}{
		{RuntimeInProcessUnit, false},
		{RuntimeProcessLocalTestcontainer, true},
		{RuntimeExternalSidecar, true},
		{RuntimeBrowserFlow, true},
		{RuntimeStaticAnalysis, false},
	}
	for _, c := range cases {
		t.Run(string(c.k), func(t *testing.T) {
			if got := c.k.RequiresTestHarness(); got != c.want {
				t.Errorf("%q.RequiresTestHarness() = %v, want %v", c.k, got, c.want)
			}
		})
	}
}

func TestRef_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		r       Ref
		wantErr string // substring; "" = expect nil
	}{
		{
			name: "valid filepath",
			r:    Ref{Type: RefFilepath, Path: "ui/e2e/agentic/x.spec.ts"},
		},
		{
			name: "valid template_id",
			r:    Ref{Type: RefTemplateID, ID: "tcp.binary-protobuf.java-junit.v1"},
		},
		{name: "invalid type", r: Ref{Type: "url", ID: "x"}, wantErr: "not one of"},
		{name: "filepath missing path", r: Ref{Type: RefFilepath}, wantErr: "non-empty path"},
		{name: "filepath sets id", r: Ref{Type: RefFilepath, Path: "x", ID: "y"}, wantErr: "must not set id"},
		{name: "template_id missing id", r: Ref{Type: RefTemplateID}, wantErr: "non-empty id"},
		{name: "template_id sets path", r: Ref{Type: RefTemplateID, ID: "x", Path: "y"}, wantErr: "must not set path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.r.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("expected error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestCheck_Validate(t *testing.T) {
	t.Parallel()

	validRef := Ref{Type: RefFilepath, Path: "x_test.go"}

	cases := []struct {
		name    string
		c       Check
		wantErr string
	}{
		{
			name: "happy path — unit",
			c: Check{
				Target:   "executor returns ToolErrorInvalidArgs for malformed args",
				Runtime:  RuntimeInProcessUnit,
				Ref:      validRef,
				Evidence: []EvidenceRule{{Kind: "test_file_exists", Args: map[string]any{"path": "x_test.go"}}},
			},
		},
		{
			name: "happy path — testcontainer",
			c: Check{
				Target:      "executor publishes correct subject on success",
				Runtime:     RuntimeProcessLocalTestcontainer,
				TestHarness: "nats-jetstream",
				TestRuntime: "go-testing-net",
				Ref:         validRef,
				Evidence:    []EvidenceRule{{Kind: "test_file_exists", Args: map[string]any{"path": "x_test.go"}}},
			},
		},
		{
			name: "happy path — sidecar",
			c: Check{
				Target:      "driver emits CS observation from real Meshtastic packet",
				Runtime:     RuntimeExternalSidecar,
				TestHarness: "meshtasticd-3.x",
				TestRuntime: "java-junit-testcontainers",
				Ref:         Ref{Type: RefTemplateID, ID: "tcp.binary-protobuf.java-junit-testcontainers.v1"},
				Evidence:    []EvidenceRule{{Kind: "surefire_passing_count", Args: map[string]any{"min": float64(1)}}},
			},
		},
		{
			name: "happy path — browser-flow",
			c: Check{
				Target:      "user creates flow via /admin/flows",
				Runtime:     RuntimeBrowserFlow,
				TestHarness: "agentic-e2e-stack",
				TestRuntime: "playwright-typescript",
				Ref:         Ref{Type: RefFilepath, Path: "ui/e2e/agentic/flow-creation.spec.ts"},
				Evidence:    []EvidenceRule{{Kind: "test_file_exists", Args: map[string]any{"path": "ui/e2e/agentic/flow-creation.spec.ts"}}},
			},
		},
		{
			name: "happy path — static-analysis (no test_harness, no test_runtime)",
			c: Check{
				Target:   "no exported function returns naked errors.New",
				Runtime:  RuntimeStaticAnalysis,
				Ref:      validRef,
				Evidence: []EvidenceRule{{Kind: "test_file_exists", Args: map[string]any{"path": "x_test.go"}}},
			},
		},
		{
			name: "happy path — unit ALLOWS test_runtime (asymmetry: test_harness forbidden, test_runtime permitted)",
			c: Check{
				Target:      "executor returns ToolErrorInvalidArgs for malformed args",
				Runtime:     RuntimeInProcessUnit,
				TestRuntime: "go-testing",
				Ref:         validRef,
				Evidence:    []EvidenceRule{{Kind: "test_file_exists", Args: map[string]any{"path": "x_test.go"}}},
			},
		},
		{
			name: "happy path — static-analysis ALLOWS test_runtime (language-specific linter)",
			c: Check{
				Target:      "no exported function returns naked errors.New",
				Runtime:     RuntimeStaticAnalysis,
				TestRuntime: "go-vet",
				Ref:         validRef,
				Evidence:    []EvidenceRule{{Kind: "test_file_exists", Args: map[string]any{"path": "x_test.go"}}},
			},
		},
		{
			// Smoke #8 run-8 negative case: architect emits a check
			// without any evidence rules. qa-reviewer (R3.7.2.j′)
			// terminates with needs_clarification because no
			// mechanical basis exists to verify the builder's tests_passing
			// claim. Validate now rejects this at the structural layer
			// so the architect retries with rules instead of shipping a
			// gate-untestable spec.
			name: "missing evidence rejected — empty Evidence slice",
			c: Check{
				Target:  "tests pass on the merged branch",
				Runtime: RuntimeInProcessUnit,
				Ref:     validRef,
				// Evidence: empty
			},
			wantErr: "evidence required",
		},
		{
			name: "evidence kind PascalCase is rejected",
			c: Check{
				Target:   "x",
				Runtime:  RuntimeInProcessUnit,
				Ref:      validRef,
				Evidence: []EvidenceRule{{Kind: "TestFileExists"}},
			},
			wantErr: "must match",
		},
		{
			name: "evidence kind with hyphen is rejected",
			c: Check{
				Target:   "x",
				Runtime:  RuntimeInProcessUnit,
				Ref:      validRef,
				Evidence: []EvidenceRule{{Kind: "test-file-exists"}},
			},
			wantErr: "must match",
		},
		{
			name: "evidence kind starting with digit is rejected",
			c: Check{
				Target:   "x",
				Runtime:  RuntimeInProcessUnit,
				Ref:      validRef,
				Evidence: []EvidenceRule{{Kind: "1test_file_exists"}},
			},
			wantErr: "must match",
		},
		{name: "missing target", c: Check{Runtime: RuntimeInProcessUnit, Ref: validRef}, wantErr: "target required"},
		{name: "invalid runtime", c: Check{Target: "x", Runtime: "wishful-thinking", Ref: validRef}, wantErr: "not a recognised RuntimeKind"},
		{
			name: "testcontainer without test_harness",
			c: Check{
				Target: "x", Runtime: RuntimeProcessLocalTestcontainer, TestRuntime: "go-testing-net", Ref: validRef,
			},
			wantErr: "requires a test_harness",
		},
		{
			name: "unit with test_harness",
			c: Check{
				Target: "x", Runtime: RuntimeInProcessUnit, TestHarness: "stub", Ref: validRef,
			},
			wantErr: "must not name a test_harness",
		},
		{
			name: "sidecar without test_runtime",
			c: Check{
				Target: "x", Runtime: RuntimeExternalSidecar, TestHarness: "stub", Ref: validRef,
			},
			wantErr: "requires a test_runtime",
		},
		{
			name: "browser-flow without test_harness",
			c: Check{
				Target: "x", Runtime: RuntimeBrowserFlow, TestRuntime: "playwright-typescript", Ref: validRef,
			},
			wantErr: "requires a test_harness",
		},
		{
			name: "evidence rule with empty kind",
			c: Check{
				Target:   "x",
				Runtime:  RuntimeInProcessUnit,
				Ref:      validRef,
				Evidence: []EvidenceRule{{Args: map[string]any{"x": "y"}}},
			},
			wantErr: "kind required",
		},
		{
			name: "ref invalid (no type)",
			c: Check{
				Target:  "x",
				Runtime: RuntimeInProcessUnit,
				Ref:     Ref{Path: "x"},
			},
			wantErr: "ref: type",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.c.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("expected error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestCheck_RoundTrip(t *testing.T) {
	t.Parallel()
	orig := &Check{
		Target:      "executor publishes graph.mutation.entity.add",
		Runtime:     RuntimeProcessLocalTestcontainer,
		TestHarness: "nats-jetstream",
		TestRuntime: "go-testing-net",
		Ref: Ref{
			Type: RefFilepath,
			Path: "cmd/semteams/sandbox/integration_test.go",
		},
		Evidence: []EvidenceRule{
			{Kind: "test_uses_build_tag", Args: map[string]any{"tag": "integration"}},
			{Kind: "test_invokes_function", Args: map[string]any{"name": "natsclient.NewTestClient"}},
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Check
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Target != orig.Target {
		t.Errorf("target: got %q, want %q", got.Target, orig.Target)
	}
	if got.Runtime != orig.Runtime {
		t.Errorf("runtime: got %q, want %q", got.Runtime, orig.Runtime)
	}
	if got.TestHarness != orig.TestHarness {
		t.Errorf("test_harness: got %q, want %q", got.TestHarness, orig.TestHarness)
	}
	if got.TestRuntime != orig.TestRuntime {
		t.Errorf("test_runtime: got %q, want %q", got.TestRuntime, orig.TestRuntime)
	}
	if got.Ref.Type != orig.Ref.Type || got.Ref.Path != orig.Ref.Path {
		t.Errorf("ref: got %+v, want %+v", got.Ref, orig.Ref)
	}
	if len(got.Evidence) != len(orig.Evidence) {
		t.Fatalf("evidence len: got %d, want %d", len(got.Evidence), len(orig.Evidence))
	}
	for i, e := range got.Evidence {
		if e.Kind != orig.Evidence[i].Kind {
			t.Errorf("evidence[%d].kind: got %q, want %q", i, e.Kind, orig.Evidence[i].Kind)
		}
	}
}

func TestCheck_Omitempty(t *testing.T) {
	t.Parallel()
	c := &Check{
		Target:  "x",
		Runtime: RuntimeInProcessUnit,
		Ref:     Ref{Type: RefFilepath, Path: "x_test.go"},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, forbidden := range []string{`"test_harness"`, `"test_runtime"`, `"evidence"`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("expected %s to be omitted when empty; body=%s", forbidden, body)
		}
	}
}

func TestCheck_Schema(t *testing.T) {
	t.Parallel()
	c := &Check{}
	s := c.Schema()
	if s.Domain != Domain {
		t.Errorf("Schema().Domain: got %q, want %q", s.Domain, Domain)
	}
	if s.Category != CategoryCheck {
		t.Errorf("Schema().Category: got %q, want %q", s.Category, CategoryCheck)
	}
	if s.Version != SchemaVersion {
		t.Errorf("Schema().Version: got %q, want %q", s.Version, SchemaVersion)
	}
}

func TestRegisterPayloads(t *testing.T) {
	t.Parallel()
	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}
	if err := RegisterPayloads(reg); err == nil {
		t.Errorf("expected duplicate-registration error on second call")
	}
}
