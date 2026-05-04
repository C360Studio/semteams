package verification

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/payloadregistry"
)

func TestApproachKind_IsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    ApproachKind
		want bool
	}{
		{ApproachInProcessUnit, true},
		{ApproachProcessLocalTestcontainer, true},
		{ApproachExternalSidecar, true},
		{ApproachBrowserFlow, true},
		{ApproachStaticAnalysis, true},
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

func TestApproachKind_RequiresHarness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    ApproachKind
		want bool
	}{
		{ApproachInProcessUnit, false},
		{ApproachProcessLocalTestcontainer, true},
		{ApproachExternalSidecar, true},
		{ApproachBrowserFlow, true},
		{ApproachStaticAnalysis, false},
	}
	for _, c := range cases {
		t.Run(string(c.k), func(t *testing.T) {
			if got := c.k.RequiresHarness(); got != c.want {
				t.Errorf("%q.RequiresHarness() = %v, want %v", c.k, got, c.want)
			}
		})
	}
}

func TestConventionRef_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		r       ConventionRef
		wantErr string // substring; "" = expect nil
	}{
		{
			name: "valid filepath",
			r:    ConventionRef{Type: ConventionFilepath, Path: "ui/e2e/agentic/x.spec.ts"},
		},
		{
			name: "valid template_id",
			r:    ConventionRef{Type: ConventionTemplateID, ID: "tcp.binary-protobuf.java-junit.v1"},
		},
		{name: "invalid type", r: ConventionRef{Type: "url", ID: "x"}, wantErr: "not one of"},
		{name: "filepath missing path", r: ConventionRef{Type: ConventionFilepath}, wantErr: "non-empty path"},
		{name: "filepath sets id", r: ConventionRef{Type: ConventionFilepath, Path: "x", ID: "y"}, wantErr: "must not set id"},
		{name: "template_id missing id", r: ConventionRef{Type: ConventionTemplateID}, wantErr: "non-empty id"},
		{name: "template_id sets path", r: ConventionRef{Type: ConventionTemplateID, ID: "x", Path: "y"}, wantErr: "must not set path"},
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

func TestCommitment_Validate(t *testing.T) {
	t.Parallel()

	validConv := ConventionRef{Type: ConventionFilepath, Path: "x_test.go"}

	cases := []struct {
		name    string
		c       Commitment
		wantErr string
	}{
		{
			name: "happy path — unit",
			c: Commitment{
				Target:     "executor returns ToolErrorInvalidArgs for malformed args",
				Approach:   ApproachInProcessUnit,
				Convention: validConv,
				Evidence:   []EvidenceRule{{Kind: "test_file_exists", Args: map[string]any{"path": "x_test.go"}}},
			},
		},
		{
			name: "happy path — testcontainer",
			c: Commitment{
				Target:     "executor publishes correct subject on success",
				Approach:   ApproachProcessLocalTestcontainer,
				Harness:    "nats-jetstream",
				Runtime:    "go-testing-net",
				Convention: validConv,
			},
		},
		{
			name: "happy path — sidecar",
			c: Commitment{
				Target:     "driver emits CS observation from real Meshtastic packet",
				Approach:   ApproachExternalSidecar,
				Harness:    "meshtasticd-3.x",
				Runtime:    "java-junit-testcontainers",
				Convention: ConventionRef{Type: ConventionTemplateID, ID: "tcp.binary-protobuf.java-junit-testcontainers.v1"},
			},
		},
		{
			name: "happy path — browser-flow",
			c: Commitment{
				Target:     "user creates flow via /admin/flows",
				Approach:   ApproachBrowserFlow,
				Harness:    "agentic-e2e-stack",
				Runtime:    "playwright-typescript",
				Convention: ConventionRef{Type: ConventionFilepath, Path: "ui/e2e/agentic/flow-creation.spec.ts"},
			},
		},
		{
			name: "happy path — static-analysis (no harness, no runtime)",
			c: Commitment{
				Target:     "no exported function returns naked errors.New",
				Approach:   ApproachStaticAnalysis,
				Convention: validConv,
			},
		},
		{
			name: "happy path — unit ALLOWS runtime (asymmetry: harness forbidden, runtime permitted)",
			c: Commitment{
				Target:     "executor returns ToolErrorInvalidArgs for malformed args",
				Approach:   ApproachInProcessUnit,
				Runtime:    "go-testing",
				Convention: validConv,
			},
		},
		{
			name: "happy path — static-analysis ALLOWS runtime (language-specific linter)",
			c: Commitment{
				Target:     "no exported function returns naked errors.New",
				Approach:   ApproachStaticAnalysis,
				Runtime:    "go-vet",
				Convention: validConv,
			},
		},
		{
			name: "evidence kind PascalCase is rejected",
			c: Commitment{
				Target:     "x",
				Approach:   ApproachInProcessUnit,
				Convention: validConv,
				Evidence:   []EvidenceRule{{Kind: "TestFileExists"}},
			},
			wantErr: "must match",
		},
		{
			name: "evidence kind with hyphen is rejected",
			c: Commitment{
				Target:     "x",
				Approach:   ApproachInProcessUnit,
				Convention: validConv,
				Evidence:   []EvidenceRule{{Kind: "test-file-exists"}},
			},
			wantErr: "must match",
		},
		{
			name: "evidence kind starting with digit is rejected",
			c: Commitment{
				Target:     "x",
				Approach:   ApproachInProcessUnit,
				Convention: validConv,
				Evidence:   []EvidenceRule{{Kind: "1test_file_exists"}},
			},
			wantErr: "must match",
		},
		{name: "missing target", c: Commitment{Approach: ApproachInProcessUnit, Convention: validConv}, wantErr: "target required"},
		{name: "invalid approach", c: Commitment{Target: "x", Approach: "wishful-thinking", Convention: validConv}, wantErr: "not a recognised ApproachKind"},
		{
			name: "testcontainer without harness",
			c: Commitment{
				Target: "x", Approach: ApproachProcessLocalTestcontainer, Runtime: "go-testing-net", Convention: validConv,
			},
			wantErr: "requires a harness",
		},
		{
			name: "unit with harness",
			c: Commitment{
				Target: "x", Approach: ApproachInProcessUnit, Harness: "stub", Convention: validConv,
			},
			wantErr: "must not name a harness",
		},
		{
			name: "sidecar without runtime",
			c: Commitment{
				Target: "x", Approach: ApproachExternalSidecar, Harness: "stub", Convention: validConv,
			},
			wantErr: "requires a runtime",
		},
		{
			name: "browser-flow without harness",
			c: Commitment{
				Target: "x", Approach: ApproachBrowserFlow, Runtime: "playwright-typescript", Convention: validConv,
			},
			wantErr: "requires a harness",
		},
		{
			name: "evidence rule with empty kind",
			c: Commitment{
				Target:     "x",
				Approach:   ApproachInProcessUnit,
				Convention: validConv,
				Evidence:   []EvidenceRule{{Args: map[string]any{"x": "y"}}},
			},
			wantErr: "kind required",
		},
		{
			name: "convention invalid (no type)",
			c: Commitment{
				Target:     "x",
				Approach:   ApproachInProcessUnit,
				Convention: ConventionRef{Path: "x"},
			},
			wantErr: "convention: type",
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

func TestCommitment_RoundTrip(t *testing.T) {
	t.Parallel()
	orig := &Commitment{
		Target:   "executor publishes graph.mutation.entity.add",
		Approach: ApproachProcessLocalTestcontainer,
		Harness:  "nats-jetstream",
		Runtime:  "go-testing-net",
		Convention: ConventionRef{
			Type: ConventionFilepath,
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
	var got Commitment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Target != orig.Target {
		t.Errorf("target: got %q, want %q", got.Target, orig.Target)
	}
	if got.Approach != orig.Approach {
		t.Errorf("approach: got %q, want %q", got.Approach, orig.Approach)
	}
	if got.Harness != orig.Harness {
		t.Errorf("harness: got %q, want %q", got.Harness, orig.Harness)
	}
	if got.Runtime != orig.Runtime {
		t.Errorf("runtime: got %q, want %q", got.Runtime, orig.Runtime)
	}
	if got.Convention.Type != orig.Convention.Type || got.Convention.Path != orig.Convention.Path {
		t.Errorf("convention: got %+v, want %+v", got.Convention, orig.Convention)
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

func TestCommitment_Omitempty(t *testing.T) {
	t.Parallel()
	c := &Commitment{
		Target:     "x",
		Approach:   ApproachInProcessUnit,
		Convention: ConventionRef{Type: ConventionFilepath, Path: "x_test.go"},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, forbidden := range []string{`"harness"`, `"runtime"`, `"evidence"`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("expected %s to be omitted when empty; body=%s", forbidden, body)
		}
	}
}

func TestCommitment_Schema(t *testing.T) {
	t.Parallel()
	c := &Commitment{}
	s := c.Schema()
	if s.Domain != Domain {
		t.Errorf("Schema().Domain: got %q, want %q", s.Domain, Domain)
	}
	if s.Category != CategoryCommitment {
		t.Errorf("Schema().Category: got %q, want %q", s.Category, CategoryCommitment)
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
