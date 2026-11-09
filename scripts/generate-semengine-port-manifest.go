//go:build ignore

// Generate the machine-verified SHA-256 provenance, MIT copyright verification,
// internal/ destination mapping, and fail-closed external-import disposition
// manifest for the 21 frozen-SemStreams units absorbed by SemTeams in Milestone 2.
//
// Standard library only: imports zero SemStreams, SemEngine, or third-party
// packages so the generator adds no framework dependency edge of its own.
//
// Usage from repository root:
//
//	go run ./scripts/generate-semengine-port-manifest.go
//	go run ./scripts/generate-semengine-port-manifest.go -format=json
//	go run ./scripts/generate-semengine-port-manifest.go -write-json docs/migrations/semengine-port-manifest/manifest.json
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	semstreamsModule          = "github.com/c360studio/semstreams"
	expectedSemStreamsVersion = "v1.0.0-beta.162.0.20260930150212-8b99efe9c66a"
	expectedSemStreamsCommit  = "8b99efe9c66a4faa4fa509f9f62cc6bad8392128"
	expectedSemStreamsSum     = "h1:WAlGLMjE/3EnYyeoqounCfYnVyIB3igMlG+FLP1Iq+o="
	semteamsModule            = "github.com/c360studio/semteams"
	semengineModule           = "github.com/c360studio/semengine"
)

type unitScanMode string

const (
	modeFullDir             unitScanMode = "full_dir"
	modeShallowDir          unitScanMode = "shallow_dir"
	modeShallowPlusTestdata unitScanMode = "shallow_plus_testdata"
	modeFileSlice           unitScanMode = "file_slice"
)

const (
	adaptVerbatimCopy       = "verbatim-copy"
	adaptVerbatimImport     = "verbatim-plus-import-rewrite"
	adaptStructural         = "structural-adaptation"
	adaptReplaceTrimmed     = "replace-with-trimmed-implementation"
	catSemEngineMain        = "semengine_main"
	catSemEngine04A06       = "semengine_setup_04a_06"
	catSemEnginePromotedPkg = "semengine_promoted_pkg"
	catSemTeamsLocalAdapter = "semteams_local_adapter"
	catDropInTrimmedGateway = "drop_in_trimmed_gateway"
	catTestOnlyRewire       = "test_only_rewire"
)

type unitSpec struct {
	ID                  string
	SourceSubpath       string
	ImportSubpath       string // empty for file_slice units whose package changes
	DestinationDir      string
	Mode                unitScanMode
	SliceFiles          []string
	SliceGlobPrefix     string
	AllowedChildSubdirs []string
	ContainerChildDirs  map[string][]string
	TargetIssue         string
	ReplaceWithTrimmed  bool
	Notes               string
}

var absorbedUnitSpecs = []unitSpec{
	{
		ID:                  "agentic",
		SourceSubpath:       "agentic",
		ImportSubpath:       "agentic",
		DestinationDir:      "internal/agentic",
		Mode:                modeShallowDir,
		AllowedChildSubdirs: []string{"agentrun", "identity", "research"},
		TargetIssue:         "#404",
		Notes:               "20 agentic payload registrations, LoopExecutionContract, LessonContract, sentinels, and DispatchEnforcedMetadataKeys; excludes agentic/{identity,research}",
	},
	{
		ID:             "agentic/agentrun",
		SourceSubpath:  "agentic/agentrun",
		ImportSubpath:  "agentic/agentrun",
		DestinationDir: "internal/agentic/agentrun",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "MilestoneSubscriber, RunProjection, and settlement reconciliation; rewires lifecyclecleanup -> semengine/pkg/lifecyclecleanup and ADR-113 Stop(ctx)",
	},
	{
		ID:              "service/milestone_slice",
		SourceSubpath:   "service",
		ImportSubpath:   "",
		DestinationDir:  "internal/agentic/milestone",
		Mode:            modeFileSlice,
		SliceFiles:      []string{"milestone_health_test.go", "milestone_service.go", "milestone_service_test.go"},
		SliceGlobPrefix: "milestone",
		TargetIssue:     "#404",
		Notes:           "MilestoneService slice cut from semengine/service (semengine#30); renames package service -> milestone and tests public Health()/HTTP mux",
	},
	{
		ID:             "vocabulary/agentic",
		SourceSubpath:  "vocabulary/agentic",
		ImportSubpath:  "vocabulary/agentic",
		DestinationDir: "internal/agentic/vocab",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Agentic predicate IRIs and Register() over semengine/vocabulary.Register",
	},
	{
		ID:             "internal/looptoken",
		SourceSubpath:  "internal/looptoken",
		ImportSubpath:  "internal/looptoken",
		DestinationDir: "internal/agentic/looptoken",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Loop capability token helper; adds required new internal/agentic/looptoken/token_test.go in #404",
	},
	{
		ID:             "persona",
		SourceSubpath:  "persona",
		ImportSubpath:  "persona",
		DestinationDir: "internal/persona",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Persona fragment loader and prompt assembler used by cmd/semteams and agentic-loop; manager_integration_test.go rewires to internal/natstest",
	},
	{
		ID:             "internal/deliverylane",
		SourceSubpath:  "internal/deliverylane",
		ImportSubpath:  "internal/deliverylane",
		DestinationDir: "internal/agentic/deliverylane",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Channel delivery-lane helper shared by agentic-dispatch and agentic-loop",
	},
	{
		ID:             "internal/agentterminal",
		SourceSubpath:  "internal/agentterminal",
		ImportSubpath:  "internal/agentterminal",
		DestinationDir: "internal/agentic/agentterminal",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Terminal outcome classifier shared by agentic-loop and agentic-dispatch",
	},
	{
		ID:                  "model/wire",
		SourceSubpath:       "model/wire",
		ImportSubpath:       "model/wire",
		DestinationDir:      "internal/agentic/modelwire",
		Mode:                modeShallowPlusTestdata,
		AllowedChildSubdirs: []string{"responses", "testdata"},
		TargetIssue:         "#404",
		Notes:               "Chat Completions + Anthropic wire adapter and golden fixtures; also backs loop LLMSummarizer after graph/llm removal",
	},
	{
		ID:                  "model/wire/responses",
		SourceSubpath:       "model/wire/responses",
		ImportSubpath:       "model/wire/responses",
		DestinationDir:      "internal/agentic/modelwire/responses",
		Mode:                modeShallowPlusTestdata,
		AllowedChildSubdirs: []string{"testdata"},
		TargetIssue:         "#404",
		Notes:               "OpenAI Responses wire adapter and golden fixtures",
	},
	{
		ID:                  "processor/agentic-loop",
		SourceSubpath:       "processor/agentic-loop",
		ImportSubpath:       "processor/agentic-loop",
		DestinationDir:      "internal/agentic/loop",
		Mode:                modeShallowDir,
		AllowedChildSubdirs: []string{"internal", "lessonmatch", "prompt"},
		ContainerChildDirs: map[string][]string{
			"internal": {"loopbucket", "looprequest"},
		},
		TargetIssue: "#404",
		Notes:       "Agentic loop orchestrator; injects ToolRegistryReader via RegisterWithToolRegistry, replaces graph/llm with modelwire, rewires graph.mutation.> and RollbackFailedStart, aligns durability with semengine#24",
	},
	{
		ID:             "processor/agentic-loop/internal/loopbucket",
		SourceSubpath:  "processor/agentic-loop/internal/loopbucket",
		ImportSubpath:  "processor/agentic-loop/internal/loopbucket",
		DestinationDir: "internal/agentic/loop/internal/loopbucket",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Loop KV bucket naming/binding helper",
	},
	{
		ID:             "processor/agentic-loop/internal/looprequest",
		SourceSubpath:  "processor/agentic-loop/internal/looprequest",
		ImportSubpath:  "processor/agentic-loop/internal/looprequest",
		DestinationDir: "internal/agentic/loop/internal/looprequest",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Loop NATS request/reply envelope helper",
	},
	{
		ID:             "processor/agentic-loop/lessonmatch",
		SourceSubpath:  "processor/agentic-loop/lessonmatch",
		ImportSubpath:  "processor/agentic-loop/lessonmatch",
		DestinationDir: "internal/agentic/loop/lessonmatch",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Role-scoped lesson retrieval matcher",
	},
	{
		ID:             "processor/agentic-loop/prompt",
		SourceSubpath:  "processor/agentic-loop/prompt",
		ImportSubpath:  "processor/agentic-loop/prompt",
		DestinationDir: "internal/agentic/loop/prompt",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Role/domain system prompt fragment registry and assembler",
	},
	{
		ID:             "processor/agentic-dispatch",
		SourceSubpath:  "processor/agentic-dispatch",
		ImportSubpath:  "processor/agentic-dispatch",
		DestinationDir: "internal/agentic/dispatch",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "User/channel message router; injects ToolRegistryReader via RegisterWithToolRegistry, rewires RollbackFailedStart and ADR-113 Stop(ctx)",
	},
	{
		ID:             "processor/agentic-model",
		SourceSubpath:  "processor/agentic-model",
		ImportSubpath:  "processor/agentic-model",
		DestinationDir: "internal/agentic/model",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "LLM endpoint caller over internal/agentic/modelwire and semengine/model; rewires RollbackFailedStart and ADR-113 Stop(ctx)",
	},
	{
		ID:                  "processor/agentic-tools",
		SourceSubpath:       "processor/agentic-tools",
		ImportSubpath:       "processor/agentic-tools",
		DestinationDir:      "internal/agentic/tools",
		Mode:                modeShallowDir,
		AllowedChildSubdirs: []string{"executors", "runner"},
		TargetIssue:         "#404",
		Notes:               "Tool execution processor + ExecutorRegistry; injects shared *ExecutorRegistry via RegisterWithExecutorRegistry, rewires graph.mutation.> and RollbackFailedStart",
	},
	{
		ID:             "processor/agentic-tools/executors",
		SourceSubpath:  "processor/agentic-tools/executors",
		ImportSubpath:  "processor/agentic-tools/executors",
		DestinationDir: "internal/agentic/tools/executors",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Built-in tool executors (BashExecutor, DecideExecutor, rules, composition, graph/web); rewires processor/rule once setup-04a-06 pins",
	},
	{
		ID:             "processor/agentic-tools/runner",
		SourceSubpath:  "processor/agentic-tools/runner",
		ImportSubpath:  "processor/agentic-tools/runner",
		DestinationDir: "internal/agentic/tools/runner",
		Mode:           modeFullDir,
		TargetIssue:    "#404",
		Notes:          "Command/sandbox execution runner behind BashExecutor and product tools",
	},
	{
		ID:                 "gateway/graph-gateway",
		SourceSubpath:      "gateway/graph-gateway",
		ImportSubpath:      "gateway/graph-gateway",
		DestinationDir:     "internal/gateway/graphgateway",
		Mode:               modeFullDir,
		TargetIssue:        "#406",
		ReplaceWithTrimmed: true,
		Notes:              "Replaced by trimmed POST /graphql gateway serving trajectory, entitiesByPrefix, pathSearch, and entity (kvRevision over kv_revision); drops globalSearch, /mcp, /inference/*, GRAPH_STATUS, graph_index_queries, and mcp_path",
	},
}

type excludedDonorSpec struct {
	Subpath string
	Reason  string
}

var excludedDonorPackages = []excludedDonorSpec{
	{
		Subpath: "agentic/identity",
		Reason:  "0 AST imports in SemTeams or the 21 absorbed units at 8b99efe9c66a; DID/credential experiment outside ADR-061 runtime scope",
	},
	{
		Subpath: "agentic/research",
		Reason:  "0 AST imports in SemTeams or the 21 absorbed units at 8b99efe9c66a (sole importer of unported pkg/fusion; used only by uncomposed research-graph-* processors)",
	},
}

var forbiddenAbsorbedImports = map[string]string{
	"agentic/identity": "excluded donor subpackage (0 SemTeams/absorbed callers)",
	"agentic/research": "excluded donor subpackage (pulls unported pkg/fusion)",
	"pkg/fusion":       "unported SemStreams package outside SemEngine admission ledger",
}

type externalImportRule struct {
	Subpath     string
	Category    string
	Replacement string
	TargetIssue string
	Notes       string
}

var externalImportCatalog = []externalImportRule{
	{Subpath: "component", Category: catSemEngineMain, Replacement: semengineModule + "/component", TargetIssue: "#404, #406", Notes: "types.PlatformMeta; Dependencies drops ToolRegistry/LifecycleManager; Stop(ctx)"},
	{Subpath: "composition", Category: catSemEngineMain, Replacement: semengineModule + "/composition", TargetIssue: "#404", Notes: "executors/composition_tools.go Validate API"},
	{Subpath: "config", Category: catSemEngineMain, Replacement: semengineModule + "/config", TargetIssue: "#404", Notes: "strict unknown-key config + Stop(ctx)"},
	{Subpath: "gateway", Category: catDropInTrimmedGateway, Replacement: "dropped in internal/gateway/graphgateway", TargetIssue: "#406", Notes: "remove dead _ gateway.Gateway = (*Component)(nil) assertion"},
	{Subpath: "graph", Category: catSemEngineMain, Replacement: semengineModule + "/graph", TargetIssue: "#404, #406", Notes: "QueryResponse 4-key envelope, ExactEntity kv_revision, EntityMutationResponse, ADR-110 Source/Timestamp"},
	{Subpath: "graph/inference", Category: catDropInTrimmedGateway, Replacement: "dropped in internal/gateway/graphgateway", TargetIssue: "#406", Notes: "anomaly review /inference/* routes excluded from trimmed SemTeams gateway"},
	{Subpath: "graph/llm", Category: catSemTeamsLocalAdapter, Replacement: semteamsModule + "/internal/agentic/modelwire + " + semengineModule + "/model", TargetIssue: "#404", Notes: "excluded at SemEngine Tier 0 (semengine#32, #484); rebind loop LLMSummarizer onto modelwire.NewClient + model.NewHTTPClient"},
	{Subpath: "graph/query", Category: catSemEngineMain, Replacement: semengineModule + "/graph/query", TargetIssue: "#406", Notes: "query envelope/classifier helpers for trimmed graphgateway"},
	{Subpath: "graph/readiness", Category: catDropInTrimmedGateway, Replacement: "dropped in internal/gateway/graphgateway", TargetIssue: "#406", Notes: "GRAPH_STATUS readiness watcher excluded with globalSearch"},
	{Subpath: "health", Category: catSemEngineMain, Replacement: semengineModule + "/health", TargetIssue: "#404", Notes: "MilestoneService health status reporting"},
	{Subpath: "internal/graphmutation", Category: catSemTeamsLocalAdapter, Replacement: semteamsModule + "/internal/agentic/graphmutation", TargetIssue: "#404", Notes: "replaces private semengine/internal/graphmutation over semengine.graph.mutation / graph.mutation.> with ADR-110 Source+Timestamp"},
	{Subpath: "internal/lifecyclecleanup", Category: catSemEnginePromotedPkg, Replacement: semengineModule + "/pkg/lifecyclecleanup", TargetIssue: "#404, #287", Notes: "RollbackFailedStart published in semengine#77 (requires non-nil rollback fn)"},
	{Subpath: "internal/semantictest", Category: catTestOnlyRewire, Replacement: "inline 6-part EntityID / 3-part Predicate helper in loop/graph_writer_test.go", TargetIssue: "#404", Notes: "1 test file only"},
	{Subpath: "message", Category: catSemEngineMain, Replacement: semengineModule + "/message", TargetIssue: "#404, #406", Notes: "BaseMessage + Triple with ADR-110 Source/Timestamp"},
	{Subpath: "metric", Category: catSemEngineMain, Replacement: semengineModule + "/metric", TargetIssue: "#404", Notes: "MetricsRegistry (semengine_ core metric prefix)"},
	{Subpath: "model", Category: catSemEngineMain, Replacement: semengineModule + "/model", TargetIssue: "#404", Notes: "RegistryReader, EndpointConfig, NewHTTPClient (track semengine#155)"},
	{Subpath: "natsclient", Category: catSemEngineMain, Replacement: semengineModule + "/natsclient (+ internal/natstest in *_test.go)", TargetIssue: "#404, #406", Notes: "WaitConsumerHandlers, RequestClassified, pkg/retry.Config; NewTestClient test sites move to internal/natstest"},
	{Subpath: "payloadbuiltins", Category: catTestOnlyRewire, Replacement: semteamsModule + "/internal/runtimecatalog.RegisterPayloads", TargetIssue: "#404", Notes: "42 absorbed *_test.go files replace aggregator registration with product catalog"},
	{Subpath: "payloadregistry", Category: catSemEngineMain, Replacement: semengineModule + "/payloadregistry", TargetIssue: "#404", Notes: "GlobalPayloadRegistry() and typed Registration"},
	{Subpath: "pkg/errs", Category: catSemEngineMain, Replacement: semengineModule + "/pkg/errs", TargetIssue: "#404, #406", Notes: "ADR-112 typed sentinel/wrapper error classification (unknown defaults to fatal)"},
	{Subpath: "pkg/graphview", Category: catSemEngineMain, Replacement: semengineModule + "/pkg/graphview", TargetIssue: "#404", Notes: "activity/view projection helpers in agentic-dispatch"},
	{Subpath: "pkg/lifecycle", Category: catSemEngineMain, Replacement: semengineModule + "/pkg/lifecycle", TargetIssue: "#404, #405", Notes: "ExecutionJournal / lifecycle contracts aligned with semengine#24"},
	{Subpath: "pkg/projection", Category: catSemEngineMain, Replacement: semengineModule + "/pkg/projection", TargetIssue: "#404", Notes: "predicate authority + MutationError helpers"},
	{Subpath: "pkg/projection/contract", Category: catSemEngineMain, Replacement: semengineModule + "/pkg/projection/contract", TargetIssue: "#404", Notes: "LoopExecutionContract and LessonContract registration"},
	{Subpath: "pkg/types", Category: catSemEngineMain, Replacement: semengineModule + "/pkg/types", TargetIssue: "#404", Notes: "EntityID and message/payload Type"},
	{Subpath: "processor/gated-dag", Category: catTestOnlyRewire, Replacement: "inline string constant in internal/agentic/entity_ids_semantics_test.go", TargetIssue: "#404", Notes: "1 test file only; uncomposed processor dropped in #399"},
	{Subpath: "processor/graph-ingest", Category: catSemEngineMain, Replacement: semengineModule + "/processor/graph-ingest", TargetIssue: "#404", Notes: "4 absorbed integration *_test.go files wire graph-ingest"},
	{Subpath: "processor/rule", Category: catSemEngine04A06, Replacement: semengineModule + "/processor/rule", TargetIssue: "#404, #405", Notes: "executors/rules.go + 2 tests; lands in SemEngine setup-04a-06"},
	{Subpath: "service", Category: catSemEngineMain, Replacement: semengineModule + "/service", TargetIssue: "#404, #406", Notes: "OpenAPI / HTTP handler interfaces"},
	{Subpath: "storage", Category: catSemEngineMain, Replacement: semengineModule + "/storage", TargetIssue: "#404", Notes: "Store interface for trajectory evidence"},
	{Subpath: "storage/storeregistry", Category: catSemEngineMain, Replacement: semengineModule + "/storage/storeregistry", TargetIssue: "#404", Notes: "ObjectStore registry lookup for oversized/evidence artifacts"},
	{Subpath: "types", Category: catSemEngineMain, Replacement: semengineModule + "/types", TargetIssue: "#404", Notes: "PlatformMeta, ComponentConfig, ServiceConfigs"},
	{Subpath: "vocabulary", Category: catSemEngineMain, Replacement: semengineModule + "/vocabulary", TargetIssue: "#404", Notes: "vocabulary.Register / RegisterPack"},
	{Subpath: "vocabulary/builtins", Category: catTestOnlyRewire, Replacement: semteamsModule + "/internal/agentic/vocab.Register()", TargetIssue: "#404", Notes: "3 absorbed integration *_test.go files replace aggregator vocab init"},
}

type newFileObligation struct {
	DestinationPath string `json:"destination_path"`
	TargetIssue     string `json:"target_issue"`
	Reason          string `json:"reason"`
}

var plannedNewFiles = []newFileObligation{
	{
		DestinationPath: "internal/agentic/graphmutation/client.go",
		TargetIssue:     "#404",
		Reason:          "Narrow NATS graph.mutation.> helper over InterfaceType semengine.graph.mutation and semengine/graph.{CreateEntityRequest,AppendTriplesRequest,EntityMutationResponse} stamping ADR-110 Source and Timestamp",
	},
	{
		DestinationPath: "internal/agentic/looptoken/token_test.go",
		TargetIssue:     "#404",
		Reason:          "Direct unit test required because upstream internal/looptoken/token.go ships with 0 *_test.go files at 8b99efe9c66a",
	},
	{
		DestinationPath: "internal/agentic/rulefamily/register.go",
		TargetIssue:     "#405",
		Reason:          "Registers publish_agent action family onto semengine/processor/rule E1-E4 extension seam without introducing a local generic execution journal ahead of semengine#24",
	},
}

type goModuleInfo struct {
	Path    string        `json:"Path"`
	Version string        `json:"Version"`
	Sum     string        `json:"Sum"`
	Dir     string        `json:"Dir"`
	Replace *goModuleInfo `json:"Replace"`
}

type licenseProvenance struct {
	SourcePath      string `json:"source_path"`
	DestinationPath string `json:"destination_path"`
	SPDXID          string `json:"spdx_id"`
	CopyrightHolder string `json:"copyright_holder"`
	Bytes           int64  `json:"bytes"`
	SHA256          string `json:"sha256"`
	NoticeVerified  bool   `json:"notice_verified"`
}

type excludedDonorSummary struct {
	SourceSubpath string `json:"source_subpath"`
	DonorProdGo   int    `json:"donor_prod_go_files"`
	DonorTestGo   int    `json:"donor_test_go_files"`
	Reason        string `json:"reason"`
}

type unitSummary struct {
	ID                  string `json:"id"`
	SourceSubpath       string `json:"source_subpath"`
	DestinationDir      string `json:"destination_dir"`
	Mode                string `json:"scan_mode"`
	TargetIssue         string `json:"target_issue"`
	UnitAdaptationClass string `json:"unit_adaptation_class"`
	ProdGoFiles         int    `json:"prod_go_files"`
	TestGoFiles         int    `json:"test_go_files"`
	OtherFiles          int    `json:"other_files"`
	TotalFiles          int    `json:"total_files"`
	TotalBytes          int64  `json:"total_bytes"`
	Notes               string `json:"notes"`
}

type fileEntry struct {
	UnitID             string   `json:"unit_id"`
	SourcePath         string   `json:"source_path"`
	DestinationPath    string   `json:"destination_path"`
	Kind               string   `json:"kind"`
	Bytes              int64    `json:"bytes"`
	SHA256             string   `json:"sha256"`
	AdaptationClass    string   `json:"adaptation_class"`
	TargetIssue        string   `json:"target_issue"`
	AdaptationTriggers []string `json:"adaptation_triggers,omitempty"`
}

type externalImportSummary struct {
	Subpath     string   `json:"subpath"`
	Category    string   `json:"category"`
	Replacement string   `json:"replacement"`
	TargetIssue string   `json:"target_issue"`
	ProdFiles   int      `json:"prod_go_files"`
	TestFiles   int      `json:"test_go_files"`
	ExampleRefs []string `json:"example_files,omitempty"`
	Notes       string   `json:"notes"`
}

type manifestDocument struct {
	SchemaVersion           int                     `json:"schema_version"`
	SourceModule            string                  `json:"source_module"`
	SourceVersion           string                  `json:"source_version"`
	SourceCommit            string                  `json:"source_commit"`
	SourceSum               string                  `json:"source_sum"`
	DestinationModule       string                  `json:"destination_module"`
	TargetSubstrateModule   string                  `json:"target_substrate_module"`
	License                 licenseProvenance       `json:"license"`
	TotalAbsorbedUnits      int                     `json:"total_absorbed_units"`
	TotalManifestFiles      int                     `json:"total_manifest_files"`
	TotalProdGoFiles        int                     `json:"total_prod_go_files"`
	TotalTestGoFiles        int                     `json:"total_test_go_files"`
	TotalNonGoFiles         int                     `json:"total_non_go_files"`
	TotalBytes              int64                   `json:"total_bytes"`
	InternalAbsorbedImports int                     `json:"internal_absorbed_import_edges"`
	ExcludedDonorPackages   []excludedDonorSummary  `json:"excluded_donor_packages"`
	NewFiles                []newFileObligation     `json:"new_files"`
	Units                   []unitSummary           `json:"units"`
	ExternalImports         []externalImportSummary `json:"external_semstreams_imports"`
	Files                   []fileEntry             `json:"files"`
}

func main() {
	var (
		repoRoot     = flag.String("root", ".", "SemTeams repository root")
		frameworkDir = flag.String("framework-dir", "", "Optional SemStreams root override for synthetic tests")
		format       = flag.String("format", "markdown", "Output format: markdown or json")
		writeJSON    = flag.String("write-json", "", "Optional path to write formatted manifest JSON")
	)
	flag.Parse()

	absRoot, err := filepath.Abs(*repoRoot)
	if err != nil {
		fatalf("resolve -root: %v", err)
	}

	modDir, modVer, modSum, err := resolveFrameworkSource(absRoot, *frameworkDir)
	if err != nil {
		fatalf("%v", err)
	}

	doc, err := buildManifest(absRoot, modDir, modVer, modSum)
	if err != nil {
		fatalf("%v", err)
	}

	if *writeJSON != "" {
		jsonBytes, marshalErr := encodeManifestJSON(doc)
		if marshalErr != nil {
			fatalf("marshal json: %v", marshalErr)
		}
		outPath := *writeJSON
		if !filepath.IsAbs(outPath) {
			outPath = filepath.Join(absRoot, outPath)
		}
		if mkErr := os.MkdirAll(filepath.Dir(outPath), 0o755); mkErr != nil {
			fatalf("mkdir %s: %v", filepath.Dir(outPath), mkErr)
		}
		if writeErr := os.WriteFile(outPath, jsonBytes, 0o644); writeErr != nil {
			fatalf("write %s: %v", outPath, writeErr)
		}
	}

	switch *format {
	case "json":
		jsonBytes, encErr := encodeManifestJSON(doc)
		if encErr != nil {
			fatalf("encode json: %v", encErr)
		}
		if _, writeErr := os.Stdout.Write(jsonBytes); writeErr != nil {
			fatalf("write stdout: %v", writeErr)
		}
	case "markdown":
		fmt.Print(renderMarkdownSummary(doc))
	default:
		fatalf("unsupported -format %q (want markdown or json)", *format)
	}
}

func encodeManifestJSON(doc *manifestDocument) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "generate-semengine-port-manifest: "+format+"\n", args...)
	os.Exit(1)
}

func resolveFrameworkSource(repoRoot, overrideDir string) (string, string, string, error) {
	if overrideDir != "" {
		absOverride, err := filepath.Abs(overrideDir)
		if err != nil {
			return "", "", "", fmt.Errorf("resolve -framework-dir: %w", err)
		}
		return absOverride, expectedSemStreamsVersion, expectedSemStreamsSum, nil
	}

	cmd := exec.Command("go", "list", "-m", "-json", semstreamsModule)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", "", fmt.Errorf("go list -m -json %s failed: %w\n%s", semstreamsModule, err, string(out))
	}

	var info goModuleInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return "", "", "", fmt.Errorf("parse go list -m output: %w", err)
	}
	if info.Replace != nil {
		return "", "", "", fmt.Errorf("go.mod replace directive active on %s (%s); manifest must measure the unmodified frozen module cache", semstreamsModule, info.Replace.Dir)
	}
	if info.Version != expectedSemStreamsVersion {
		return "", "", "", fmt.Errorf("unexpected %s version %q (want %q)", semstreamsModule, info.Version, expectedSemStreamsVersion)
	}
	if info.Sum != expectedSemStreamsSum {
		return "", "", "", fmt.Errorf("unexpected %s sum %q (want %q)", semstreamsModule, info.Sum, expectedSemStreamsSum)
	}
	if info.Dir == "" {
		return "", "", "", fmt.Errorf("%s module cache directory is empty; run go mod download", semstreamsModule)
	}
	return info.Dir, info.Version, info.Sum, nil
}

func verifyMITLicense(modDir string) (licenseProvenance, fileEntry, error) {
	licPath := filepath.Join(modDir, "LICENSE")
	raw, err := os.ReadFile(licPath)
	if err != nil {
		return licenseProvenance{}, fileEntry{}, fmt.Errorf("read upstream LICENSE (%s): %w", licPath, err)
	}
	text := string(raw)
	requiredSnippets := []string{
		"MIT License",
		"Copyright (c) 2025 C360",
		"Permission is hereby granted, free of charge",
		"The above copyright notice and this permission notice shall be included in all",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			return licenseProvenance{}, fileEntry{}, fmt.Errorf("upstream LICENSE missing required MIT provenance notice %q", snippet)
		}
	}
	sum := sha256Hex(raw)
	size := int64(len(raw))
	prov := licenseProvenance{
		SourcePath:      "LICENSE",
		DestinationPath: "internal/agentic/LICENSE",
		SPDXID:          "MIT",
		CopyrightHolder: "Copyright (c) 2025 C360",
		Bytes:           size,
		SHA256:          sum,
		NoticeVerified:  true,
	}
	entry := fileEntry{
		UnitID:          "license",
		SourcePath:      "LICENSE",
		DestinationPath: "internal/agentic/LICENSE",
		Kind:            "license",
		Bytes:           size,
		SHA256:          sum,
		AdaptationClass: adaptVerbatimCopy,
		TargetIssue:     "#404",
	}
	return prov, entry, nil
}

func buildManifest(repoRoot, modDir, modVer, modSum string) (*manifestDocument, error) {
	licProv, licEntry, err := verifyMITLicense(modDir)
	if err != nil {
		return nil, err
	}

	absorbedImportSubpaths := make(map[string]string, len(absorbedUnitSpecs))
	for _, u := range absorbedUnitSpecs {
		if u.ImportSubpath != "" {
			absorbedImportSubpaths[u.ImportSubpath] = u.DestinationDir
		}
	}

	extRuleBySubpath := make(map[string]externalImportRule, len(externalImportCatalog))
	extCounts := make(map[string]*externalImportSummary, len(externalImportCatalog))
	for _, r := range externalImportCatalog {
		extRuleBySubpath[r.Subpath] = r
		extCounts[r.Subpath] = &externalImportSummary{
			Subpath:     r.Subpath,
			Category:    r.Category,
			Replacement: r.Replacement,
			TargetIssue: r.TargetIssue,
			Notes:       r.Notes,
		}
	}

	if err := checkRepoForbiddenImports(repoRoot); err != nil {
		return nil, err
	}

	var (
		files                   = []fileEntry{licEntry}
		units                   = make([]unitSummary, 0, len(absorbedUnitSpecs))
		totalProdGo             int
		totalTestGo             int
		totalNonGo              = 1 // LICENSE
		totalBytes              = licEntry.Bytes
		internalAbsorbedImports int
		forbiddenViolations     []string
		unclassifiedImports     []string
	)

	fset := token.NewFileSet()

	for _, spec := range absorbedUnitSpecs {
		relPaths, scanErr := collectUnitFiles(modDir, spec)
		if scanErr != nil {
			return nil, scanErr
		}
		sort.Strings(relPaths)

		var (
			uSum          unitSummary
			hasStructural bool
			hasImportRewr bool
		)
		uSum.ID = spec.ID
		uSum.SourceSubpath = spec.SourceSubpath
		uSum.DestinationDir = spec.DestinationDir
		uSum.Mode = string(spec.Mode)
		uSum.TargetIssue = spec.TargetIssue
		uSum.Notes = spec.Notes

		for _, relSrc := range relPaths {
			fullPath := filepath.Join(modDir, filepath.FromSlash(relSrc))
			raw, readErr := os.ReadFile(fullPath)
			if readErr != nil {
				return nil, fmt.Errorf("read %s: %w", relSrc, readErr)
			}
			relWithinUnit := strings.TrimPrefix(relSrc, spec.SourceSubpath+"/")
			destPath := spec.DestinationDir + "/" + relWithinUnit
			kind := classifyFileKind(relWithinUnit)
			size := int64(len(raw))
			sum := sha256Hex(raw)

			var triggers []string
			adaptClass := adaptVerbatimCopy

			if strings.HasSuffix(relSrc, ".go") {
				isTest := strings.HasSuffix(relSrc, "_test.go")
				if isTest {
					uSum.TestGoFiles++
					totalTestGo++
				} else {
					uSum.ProdGoFiles++
					totalProdGo++
				}

				parsed, parseErr := parser.ParseFile(fset, fullPath, raw, parser.ImportsOnly)
				if parseErr != nil {
					return nil, fmt.Errorf("parse %s: %w", relSrc, parseErr)
				}

				hasSemStreamsImport := false
				seenSubpathsInFile := make(map[string]bool)
				for _, imp := range parsed.Imports {
					impPath, unqErr := strconv.Unquote(imp.Path.Value)
					if unqErr != nil {
						return nil, fmt.Errorf("unquote import in %s: %w", relSrc, unqErr)
					}
					if impPath == semstreamsModule {
						hasSemStreamsImport = true
						unclassifiedImports = append(unclassifiedImports, fmt.Sprintf("%s -> %s (root module import)", relSrc, impPath))
						continue
					}
					if !strings.HasPrefix(impPath, semstreamsModule+"/") {
						continue
					}
					hasSemStreamsImport = true
					sub := strings.TrimPrefix(impPath, semstreamsModule+"/")
					if reason, forbidden := forbiddenAbsorbedImports[sub]; forbidden {
						forbiddenViolations = append(forbiddenViolations, fmt.Sprintf("%s imports forbidden %s/%s (%s)", relSrc, semstreamsModule, sub, reason))
						continue
					}
					if _, internalOK := absorbedImportSubpaths[sub]; internalOK {
						internalAbsorbedImports++
						continue
					}
					rule, classified := extRuleBySubpath[sub]
					if !classified {
						unclassifiedImports = append(unclassifiedImports, fmt.Sprintf("%s -> %s", relSrc, impPath))
						continue
					}
					if rule.Category == catTestOnlyRewire && !isTest {
						unclassifiedImports = append(unclassifiedImports, fmt.Sprintf("%s -> %s (test_only_rewire imported by non-test file)", relSrc, impPath))
						continue
					}
					if !seenSubpathsInFile[sub] {
						seenSubpathsInFile[sub] = true
						rec := extCounts[sub]
						if isTest {
							rec.TestFiles++
						} else {
							rec.ProdFiles++
						}
						if len(rec.ExampleRefs) < 3 {
							rec.ExampleRefs = append(rec.ExampleRefs, relSrc)
						}
					}
					if trig := triggerForExternalSubpath(sub); trig != "" {
						triggers = appendUnique(triggers, trig)
					}
				}

				adaptClass, triggers = finalizeGoAdaptation(spec, relSrc, raw, isTest, hasSemStreamsImport, triggers)
				switch adaptClass {
				case adaptStructural:
					hasStructural = true
				case adaptVerbatimImport:
					hasImportRewr = true
				}
			} else {
				uSum.OtherFiles++
				totalNonGo++
				if spec.ReplaceWithTrimmed {
					adaptClass = adaptReplaceTrimmed
				} else {
					adaptClass = adaptVerbatimCopy
				}
			}

			uSum.TotalFiles++
			uSum.TotalBytes += size
			totalBytes += size

			files = append(files, fileEntry{
				UnitID:             spec.ID,
				SourcePath:         relSrc,
				DestinationPath:    destPath,
				Kind:               kind,
				Bytes:              size,
				SHA256:             sum,
				AdaptationClass:    adaptClass,
				TargetIssue:        spec.TargetIssue,
				AdaptationTriggers: triggers,
			})
		}

		switch {
		case spec.ReplaceWithTrimmed:
			uSum.UnitAdaptationClass = adaptReplaceTrimmed
		case hasStructural:
			uSum.UnitAdaptationClass = adaptStructural
		case hasImportRewr:
			uSum.UnitAdaptationClass = adaptVerbatimImport
		default:
			uSum.UnitAdaptationClass = adaptVerbatimCopy
		}

		units = append(units, uSum)
	}

	if len(forbiddenViolations) > 0 {
		sort.Strings(forbiddenViolations)
		return nil, fmt.Errorf("forbidden SemStreams import(s) detected:\n  %s", strings.Join(forbiddenViolations, "\n  "))
	}
	if len(unclassifiedImports) > 0 {
		sort.Strings(unclassifiedImports)
		return nil, fmt.Errorf("unclassified SemStreams import(s) in absorbed units:\n  %s", strings.Join(unclassifiedImports, "\n  "))
	}

	excludedSummaries, err := summarizeExcludedDonorPackages(modDir)
	if err != nil {
		return nil, err
	}

	extList := make([]externalImportSummary, 0, len(externalImportCatalog))
	for _, r := range externalImportCatalog {
		rec := extCounts[r.Subpath]
		if rec.ProdFiles == 0 && rec.TestFiles == 0 {
			continue
		}
		extList = append(extList, *rec)
	}
	sort.Slice(extList, func(i, j int) bool {
		return extList[i].Subpath < extList[j].Subpath
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].DestinationPath < files[j].DestinationPath
	})

	newFiles := make([]newFileObligation, len(plannedNewFiles))
	copy(newFiles, plannedNewFiles)
	sort.Slice(newFiles, func(i, j int) bool {
		return newFiles[i].DestinationPath < newFiles[j].DestinationPath
	})

	return &manifestDocument{
		SchemaVersion:           1,
		SourceModule:            semstreamsModule,
		SourceVersion:           modVer,
		SourceCommit:            expectedSemStreamsCommit,
		SourceSum:               modSum,
		DestinationModule:       semteamsModule,
		TargetSubstrateModule:   semengineModule,
		License:                 licProv,
		TotalAbsorbedUnits:      len(units),
		TotalManifestFiles:      len(files),
		TotalProdGoFiles:        totalProdGo,
		TotalTestGoFiles:        totalTestGo,
		TotalNonGoFiles:         totalNonGo,
		TotalBytes:              totalBytes,
		InternalAbsorbedImports: internalAbsorbedImports,
		ExcludedDonorPackages:   excludedSummaries,
		NewFiles:                newFiles,
		Units:                   units,
		ExternalImports:         extList,
		Files:                   files,
	}, nil
}

func checkRepoForbiddenImports(repoRoot string) error {
	fset := token.NewFileSet()
	skipDirs := map[string]bool{
		".git":         true,
		".worktrees":   true,
		"node_modules": true,
		"vendor":       true,
		".svelte-kit":  true,
	}
	var violations []string
	walkErr := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != repoRoot && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "scripts/") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return fmt.Errorf("parse repo file %s: %w", rel, parseErr)
		}
		for _, imp := range parsed.Imports {
			impPath, unqErr := strconv.Unquote(imp.Path.Value)
			if unqErr != nil {
				continue
			}
			if !strings.HasPrefix(impPath, semstreamsModule+"/") {
				continue
			}
			sub := strings.TrimPrefix(impPath, semstreamsModule+"/")
			if reason, forbidden := forbiddenAbsorbedImports[sub]; forbidden {
				violations = append(violations, fmt.Sprintf("repo file %s imports forbidden %s/%s (%s)", rel, semstreamsModule, sub, reason))
			}
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("forbidden SemStreams import(s) detected:\n  %s", strings.Join(violations, "\n  "))
	}
	return nil
}

func collectUnitFiles(modDir string, spec unitSpec) ([]string, error) {
	unitDir := filepath.Join(modDir, filepath.FromSlash(spec.SourceSubpath))
	entries, err := os.ReadDir(unitDir)
	if err != nil {
		return nil, fmt.Errorf("read unit directory %s: %w", spec.SourceSubpath, err)
	}

	switch spec.Mode {
	case modeFileSlice:
		wantSet := make(map[string]bool, len(spec.SliceFiles))
		for _, f := range spec.SliceFiles {
			wantSet[f] = true
		}
		var out []string
		for _, ent := range entries {
			name := ent.Name()
			if spec.SliceGlobPrefix != "" && strings.HasPrefix(name, spec.SliceGlobPrefix) && !wantSet[name] {
				return nil, fmt.Errorf("unaccounted %s* entry %s/%s outside declared slice", spec.SliceGlobPrefix, spec.SourceSubpath, name)
			}
			if wantSet[name] {
				if ent.IsDir() {
					return nil, fmt.Errorf("expected file %s/%s, found directory", spec.SourceSubpath, name)
				}
				out = append(out, spec.SourceSubpath+"/"+name)
				delete(wantSet, name)
			}
		}
		if len(wantSet) > 0 {
			missing := make([]string, 0, len(wantSet))
			for k := range wantSet {
				missing = append(missing, spec.SourceSubpath+"/"+k)
			}
			sort.Strings(missing)
			return nil, fmt.Errorf("missing expected slice file(s): %s", strings.Join(missing, ", "))
		}
		return out, nil

	case modeFullDir:
		var out []string
		for _, ent := range entries {
			if ent.IsDir() {
				return nil, fmt.Errorf("unaccounted subdirectory %s/%s in full_dir unit", spec.SourceSubpath, ent.Name())
			}
			out = append(out, spec.SourceSubpath+"/"+ent.Name())
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("unit %s matched 0 files", spec.SourceSubpath)
		}
		return out, nil

	case modeShallowDir, modeShallowPlusTestdata:
		allowedDirs := make(map[string]bool, len(spec.AllowedChildSubdirs))
		for _, d := range spec.AllowedChildSubdirs {
			allowedDirs[d] = true
		}
		var out []string
		for _, ent := range entries {
			name := ent.Name()
			if ent.IsDir() {
				if !allowedDirs[name] {
					return nil, fmt.Errorf("unaccounted subdirectory %s/%s in %s", spec.SourceSubpath, name, spec.Mode)
				}
				if expectedGrandchildren, hasContainer := spec.ContainerChildDirs[name]; hasContainer {
					if err := verifyContainerDir(filepath.Join(unitDir, name), spec.SourceSubpath+"/"+name, expectedGrandchildren); err != nil {
						return nil, err
					}
				}
				if spec.Mode == modeShallowPlusTestdata && name == "testdata" {
					tdEntries, tdErr := os.ReadDir(filepath.Join(unitDir, "testdata"))
					if tdErr != nil {
						return nil, fmt.Errorf("read %s/testdata: %w", spec.SourceSubpath, tdErr)
					}
					for _, tdEnt := range tdEntries {
						if tdEnt.IsDir() {
							return nil, fmt.Errorf("unaccounted nested directory %s/testdata/%s", spec.SourceSubpath, tdEnt.Name())
						}
						out = append(out, spec.SourceSubpath+"/testdata/"+tdEnt.Name())
					}
				}
				continue
			}
			out = append(out, spec.SourceSubpath+"/"+name)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("unit %s matched 0 files", spec.SourceSubpath)
		}
		return out, nil

	default:
		return nil, errors.New("unknown scan mode: " + string(spec.Mode))
	}
}

func verifyContainerDir(dirPath, relLabel string, expectedDirs []string) error {
	allowed := make(map[string]bool, len(expectedDirs))
	for _, d := range expectedDirs {
		allowed[d] = true
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read container dir %s: %w", relLabel, err)
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			return fmt.Errorf("unaccounted file %s/%s in container directory", relLabel, ent.Name())
		}
		if !allowed[ent.Name()] {
			return fmt.Errorf("unaccounted subdirectory %s/%s in container directory", relLabel, ent.Name())
		}
	}
	return nil
}

func summarizeExcludedDonorPackages(modDir string) ([]excludedDonorSummary, error) {
	out := make([]excludedDonorSummary, 0, len(excludedDonorPackages))
	for _, ex := range excludedDonorPackages {
		dirPath := filepath.Join(modDir, filepath.FromSlash(ex.Subpath))
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				out = append(out, excludedDonorSummary{
					SourceSubpath: ex.Subpath,
					Reason:        ex.Reason,
				})
				continue
			}
			return nil, fmt.Errorf("read excluded donor dir %s: %w", ex.Subpath, err)
		}
		var prodCount, testCount int
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") {
				continue
			}
			if strings.HasSuffix(ent.Name(), "_test.go") {
				testCount++
			} else {
				prodCount++
			}
		}
		out = append(out, excludedDonorSummary{
			SourceSubpath: ex.Subpath,
			DonorProdGo:   prodCount,
			DonorTestGo:   testCount,
			Reason:        ex.Reason,
		})
	}
	return out, nil
}

func classifyFileKind(relWithinUnit string) string {
	if strings.HasPrefix(relWithinUnit, "testdata/") {
		return "testdata"
	}
	if strings.HasSuffix(relWithinUnit, "_test.go") {
		return "go_test"
	}
	if strings.HasSuffix(relWithinUnit, ".go") {
		return "go_prod"
	}
	return "doc"
}

func triggerForExternalSubpath(sub string) string {
	switch sub {
	case "internal/lifecyclecleanup":
		return "seam:pkg_lifecyclecleanup"
	case "graph/llm":
		return "seam:graph_llm_to_modelwire"
	case "internal/graphmutation":
		return "seam:semengine_graph_mutation"
	case "processor/rule":
		return "seam:processor_rule_setup_04a_06"
	case "payloadbuiltins":
		return "test:runtimecatalog_payloads"
	case "vocabulary/builtins":
		return "test:agentic_vocab_register"
	case "processor/gated-dag":
		return "test:inline_gated_dag_const"
	case "internal/semantictest":
		return "test:inline_semantictest_helper"
	default:
		return ""
	}
}

func finalizeGoAdaptation(spec unitSpec, relSrc string, raw []byte, isTest, hasSemStreamsImport bool, triggers []string) (string, []string) {
	if spec.ReplaceWithTrimmed {
		return adaptReplaceTrimmed, nil
	}
	if spec.ID == "service/milestone_slice" {
		triggers = appendUnique(triggers, "package_rename:service_to_milestone")
		if relSrc == "service/milestone_health_test.go" {
			triggers = appendUnique(triggers, "test:public_health_endpoint")
		}
	}
	if bytes.Contains(raw, []byte(".ToolRegistry")) {
		triggers = appendUnique(triggers, "seam:executor_registry_di")
	}
	if isTest && (bytes.Contains(raw, []byte("NewTestClient")) || bytes.Contains(raw, []byte("natsclient.TestClient"))) {
		triggers = appendUnique(triggers, "test:internal_natstest")
	}
	if bytes.Contains(raw, []byte("PlatformMeta")) && bytes.Contains(raw, []byte("component.PlatformMeta")) {
		triggers = appendUnique(triggers, "seam:types_platform_meta")
	}
	sort.Strings(triggers)
	if len(triggers) > 0 {
		return adaptStructural, triggers
	}
	if hasSemStreamsImport {
		return adaptVerbatimImport, nil
	}
	return adaptVerbatimCopy, nil
}

func appendUnique(list []string, val string) []string {
	for _, existing := range list {
		if existing == val {
			return list
		}
	}
	return append(list, val)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func renderMarkdownSummary(doc *manifestDocument) string {
	var b strings.Builder

	b.WriteString("### Provenance summary\n\n")
	fmt.Fprintf(&b, "- Frozen donor module: `%s %s` (`%s`, `%s`)\n",
		doc.SourceModule, doc.SourceVersion, doc.SourceCommit, doc.SourceSum)
	fmt.Fprintf(&b, "- Upstream MIT `LICENSE`: `%s` -> `%s` (`%d` bytes, SHA-256 `%s`, `%s`, verified: `%t`)\n",
		doc.License.SourcePath, doc.License.DestinationPath, doc.License.Bytes, doc.License.SHA256, doc.License.CopyrightHolder, doc.License.NoticeVerified)
	fmt.Fprintf(&b, "- Absorbed units: **%d** units, **%d** manifest files (**%d** prod `.go`, **%d** `_test.go`, **%d** non-Go docs/fixtures/license; **%d** bytes total)\n",
		doc.TotalAbsorbedUnits, doc.TotalManifestFiles, doc.TotalProdGoFiles, doc.TotalTestGoFiles, doc.TotalNonGoFiles, doc.TotalBytes)
	fmt.Fprintf(&b, "- Internal cross-unit import edges rewritten to `%s/internal/...`: **%d**\n",
		doc.DestinationModule, doc.InternalAbsorbedImports)
	fmt.Fprintf(&b, "- Required new Milestone 2 files tracked alongside absorbed units: **%d** (`%s`)\n\n",
		len(doc.NewFiles), joinNewFilePaths(doc.NewFiles))

	b.WriteString("### 21 absorbed units (`8b99efe9c66a` -> `internal/...`)\n\n")
	b.WriteString("| Unit | Frozen SemStreams source | SemTeams `internal/` destination | Prod `.go` | `_test.go` | Non-Go | Bytes | Adaptation class | Issue |\n")
	b.WriteString("| --- | --- | --- | ---: | ---: | ---: | ---: | --- | --- |\n")
	for _, u := range doc.Units {
		srcLabel := "`" + u.SourceSubpath + "`"
		if u.Mode == string(modeFileSlice) {
			srcLabel = "`service/milestone_*.go`"
		}
		fmt.Fprintf(&b, "| `%s` | %s | `%s` | %d | %d | %d | %d | `%s` | `%s` |\n",
			u.ID, srcLabel, u.DestinationDir, u.ProdGoFiles, u.TestGoFiles, u.OtherFiles, u.TotalBytes, u.UnitAdaptationClass, u.TargetIssue)
	}

	b.WriteString("\n### Excluded donor subpackages under `agentic/` (zero-import verified)\n\n")
	b.WriteString("| Excluded donor subpath | Donor prod `.go` | Donor `_test.go` | Disposition |\n")
	b.WriteString("| --- | ---: | ---: | --- |\n")
	for _, ex := range doc.ExcludedDonorPackages {
		fmt.Fprintf(&b, "| `%s` | %d | %d | Excluded — %s |\n",
			ex.SourceSubpath, ex.DonorProdGo, ex.DonorTestGo, ex.Reason)
	}

	b.WriteString("\n### Required new Milestone 2 files\n\n")
	b.WriteString("| Destination file | Issue | Obligation |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, nf := range doc.NewFiles {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", nf.DestinationPath, nf.TargetIssue, nf.Reason)
	}

	b.WriteString("\n### External `semstreams/...` imports across absorbed units (34 paths, fail-closed)\n\n")
	b.WriteString("| SemStreams import subpath | Absorbed prod `.go` | Absorbed `_test.go` | Category | Milestone 2 target / replacement | Issue |\n")
	b.WriteString("| --- | ---: | ---: | --- | --- | --- |\n")
	for _, ext := range doc.ExternalImports {
		fmt.Fprintf(&b, "| `%s` | %d | %d | `%s` | `%s` | `%s` |\n",
			ext.Subpath, ext.ProdFiles, ext.TestFiles, ext.Category, ext.Replacement, ext.TargetIssue)
	}

	return b.String()
}

func joinNewFilePaths(items []newFileObligation) string {
	paths := make([]string, 0, len(items))
	for _, it := range items {
		paths = append(paths, it.DestinationPath)
	}
	return strings.Join(paths, "`, `")
}
