//go:build ignore

// Measure SemStreams import paths across SemTeams Go source files and emit the
// Milestone 1/2 SemEngine crosswalk table or machine-readable JSON summary.
//
// Standard library only: imports zero SemStreams or third-party packages so the
// measurement tool adds no framework import edge of its own.
//
// Usage from repository root:
//
//	go run ./scripts/measure-semengine-port-ledger.go
//	go run ./scripts/measure-semengine-port-ledger.go -format=json
//	go run ./scripts/measure-semengine-port-ledger.go -active-only
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const semstreamsModule = "github.com/c360studio/semstreams"

const (
	groupSemEngineMain   = "1. SemEngine main"
	groupSemEngine04A06  = "2. SemEngine setup-04a-06"
	groupSemTeamsAbsorb  = "3. SemTeams Milestone 2 absorb"
	groupMilestone1Prune = "4. Milestone 1 drop/decouple"
)

type catalogEntry struct {
	Subpath              string
	Group                string
	SemEngineDisposition string
	PostPortHome         string
	UpstreamRefs         string
	ConsumerAction       string
}

var ledgerCatalog = []catalogEntry{
	// Group 1: SemEngine present today on main (20 paths kept by SemTeams)
	{
		Subpath:              "component",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/component",
		UpstreamRefs:         "ADR-108, ADR-113, semengine#29",
		ConsumerAction:       "M2 (#404, #407): rewire imports; Registry.Register/ListFactories in catalog_test.go; types.PlatformMeta; Dependencies drops ToolRegistry/LifecycleManager",
	},
	{
		Subpath:              "composition",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "repair-before-port",
		PostPortHome:         "semengine/composition",
		UpstreamRefs:         "ADR-100",
		ConsumerAction:       "M2 (#407): replace composition.AssertValid(t, registry, cfg) with composition.Validate(registry, cfg) (*Result, error) in test/contract/composition_contract_test.go",
	},
	{
		Subpath:              "config",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/config",
		UpstreamRefs:         "ADR-104, ADR-113",
		ConsumerAction:       "M2 (#407): Manager.Stop(ctx); SEMENGINE_ env prefix and semengine_config_<org>_<stem> bucket; strict unknown-key rejection",
	},
	{
		Subpath:              "graph",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/graph",
		UpstreamRefs:         "ADR-109, ADR-110",
		ConsumerAction:       "M2 (#406, #407): NewExactEntityReader returns (*ExactEntityReader, error); ExactEntity json kv_revision; EntityMutationResponse + ReconcilePredicatesRequest.Source (and per-triple Source/Timestamp)",
	},
	{
		Subpath:              "graph/inference",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/graph/inference",
		UpstreamRefs:         "semengine#97",
		ConsumerAction:       "M1 (#399) / M2 (#402, #406): hierarchy slice on main; anomaly/detector types stay out of trimmed SemTeams graph-gateway",
	},
	{
		Subpath:              "message",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/message",
		UpstreamRefs:         "ADR-110",
		ConsumerAction:       "M2 (#404, #407): rewire imports; stream-lane statements require Source and Timestamp",
	},
	{
		Subpath:              "metric",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/metric",
		UpstreamRefs:         "semengine#69",
		ConsumerAction:       "M2 (#407): rewire cmd/semteams/main.go import; core metrics use semengine_ namespace",
	},
	{
		Subpath:              "model",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/model",
		UpstreamRefs:         "semengine#155",
		ConsumerAction:       "M2 (#404, #407): rewire test/runtime imports; track semengine#155 reader/endpoint refactor before pin",
	},
	{
		Subpath:              "natsclient",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/natsclient",
		UpstreamRefs:         "ADR-110, ADR-112",
		ConsumerAction:       "M1 (#400, done): NewTestClient/TestClient replaced by internal/natstest; M2 (#404, #407): rewire imports (incl. internal/natstest), use pkg/retry.Config and WaitConsumerHandlers",
	},
	{
		Subpath:              "payloadregistry",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/payloadregistry",
		UpstreamRefs:         "semengine#219",
		ConsumerAction:       "M2 (#404, #407): rewire imports; GetRegistration/IndexingProfileFor take pkg/types.Type where called",
	},
	{
		Subpath:              "pkg/errs",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/pkg/errs",
		UpstreamRefs:         "ADR-112, semengine#146, semengine#167",
		ConsumerAction:       "M2 (#404, #407): rewire imports; Classify defaults unknown errors to fatal and classifies by sentinel/type only",
	},
	{
		Subpath:              "pkg/lifecycle",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/pkg/lifecycle",
		UpstreamRefs:         "ADR-110, ADR-111, semengine#24",
		ConsumerAction:       "M2 (#404, #405, #407): rewire imports; bind lifecycle.Manager onto rule processor at host construction; align with semengine#24",
	},
	{
		Subpath:              "pkg/projection",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "repair-before-port",
		PostPortHome:         "semengine/pkg/projection",
		UpstreamRefs:         "ADR-110, ADR-112",
		ConsumerAction:       "M2 (#407): rewire imports; MutationError over ErrNoResponders carries transient Class field while errs.IsTransient stays false",
	},
	{
		Subpath:              "pkg/types",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/pkg/types",
		UpstreamRefs:         "ADR-102, ADR-104",
		ConsumerAction:       "M2 (#404, #407): rewire imports; 6-part entity ID and UTF-8 Type.Validate contract unchanged for SemTeams",
	},
	{
		Subpath:              "processor/graph-ingest",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "repair-before-port",
		PostPortHome:         "semengine/processor/graph-ingest",
		UpstreamRefs:         "ADR-108, ADR-109, ADR-110, ADR-113",
		ConsumerAction:       "M2 (#407): rewire internal/runtimecatalog registration; enforces ADR-110 single write seam",
	},
	{
		Subpath:              "processor/graph-query",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/processor/graph-query",
		UpstreamRefs:         "ADR-108, ADR-109, ADR-113, semengine#9",
		ConsumerAction:       "M2 (#406, #407): rewire catalog; adopt MIGRATION.md 4-key QueryResponse envelope, kv_revision, and COMMUNITY_INDEX readiness behavior",
	},
	{
		Subpath:              "service",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/service",
		UpstreamRefs:         "ADR-109, ADR-113, semengine#29, semengine#30, semengine#361",
		ConsumerAction:       "M1 (#401, done): logging.go log-forwarder exclude_sources kept product-local without pkg/logging; M2 (#404, #407): absorb MilestoneService, pass signalCtx to StartAll, update NewBaseServiceWithOptions/MaybeStartPProf/ComponentManager",
	},
	{
		Subpath:              "storage/objectstore",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/storage/objectstore",
		UpstreamRefs:         "ADR-063, ADR-108, ADR-112, ADR-113",
		ConsumerAction:       "M2 (#407): rewire internal/runtimecatalog and cmd/semteams imports",
	},
	{
		Subpath:              "types",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/types",
		UpstreamRefs:         "ADR-104",
		ConsumerAction:       "M2 (#404, #407): rewire imports; ServiceConfigs named map type in Manager.ConfigureFromServices",
	},
	{
		Subpath:              "vocabulary",
		Group:                groupSemEngineMain,
		SemEngineDisposition: "adapt",
		PostPortHome:         "semengine/vocabulary",
		UpstreamRefs:         "semengine#69, semengine#72, semengine#234",
		ConsumerAction:       "M1 (#401, done): RegisterPredicate(meta) in cmd/semteams/vocab/vocab.go replaced with vocabulary.Register; M2 (#407): rewire imports to semengine/vocabulary",
	},

	// Group 2: SemEngine lands in setup-04a-06 (2 paths)
	{
		Subpath:              "processor/rule",
		Group:                groupSemEngine04A06,
		SemEngineDisposition: "unported (setup-04a-06)",
		PostPortHome:         "semengine/processor/rule + semteams agent actions",
		UpstreamRefs:         "ADR-108, ADR-109, ADR-113, semengine#9, semengine#25, semengine#26, semengine#27, semengine#28",
		ConsumerAction:       "M2 (#405, #407, #287): parked until setup-04a-06 lands after graph-clustering; register publish_agent action family via E1-E3 seam",
	},
	{
		Subpath:              "processor/rule/expression",
		Group:                groupSemEngine04A06,
		SemEngineDisposition: "unported (setup-04a-06)",
		PostPortHome:         "semengine/processor/rule/expression",
		UpstreamRefs:         "semengine#9, semengine#25",
		ConsumerAction:       "M2 (#405, #407): rewire 3 rule-pack contract test files once setup-04a-06 lands",
	},

	// Group 3: SemTeams absorbs on Milestone 2 branch under ADR-061 (11 direct paths)
	{
		Subpath:              "agentic",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "defer-exclude",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#402) design; M2 (#404, #287): absorb contracts, payloads, and projection contracts from 8b99efe9",
	},
	{
		Subpath:              "agentic/agentrun",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "defer-exclude",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "semengine#8 (03B D4), semengine#30",
		ConsumerAction:       "M1 (#402) design; M2 (#404, #287): absorb run projection + MilestoneService from 8b99efe9",
	},
	{
		Subpath:              "gateway/graph-gateway",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "defer-exclude",
		PostPortHome:         "semteams trimmed graph-gateway",
		UpstreamRefs:         "semengine#8 (03B D4), graph-query/MIGRATION.md",
		ConsumerAction:       "M1 (#402) design; M2 (#406): replace with trimmed SemTeams GraphQL gateway over graph.QueryVerbs() and kv_revision",
	},
	{
		Subpath:              "persona",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#402) design; M2 (#404): absorb persona manager/fragments loader from 8b99efe9",
	},
	{
		Subpath:              "processor/agentic-dispatch",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "ADR-108, ADR-112, ADR-113, semengine#77",
		ConsumerAction:       "M1 (#402) design; M2 (#404, #287): absorb component + internal/deliverylane + model/wire; adopt ADR-112/113 + pkg/lifecyclecleanup",
	},
	{
		Subpath:              "processor/agentic-loop",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "ADR-108, ADR-111, ADR-112, ADR-113, semengine#24, semengine#77",
		ConsumerAction:       "M1 (#402) design; M2 (#404, #287): absorb loop + internal/{looptoken,agentterminal}; align durability with semengine#24",
	},
	{
		Subpath:              "processor/agentic-model",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "ADR-108, ADR-112, ADR-113, semengine#77",
		ConsumerAction:       "M1 (#402) design; M2 (#404, #287): absorb model processor + model/wire; adopt pkg/lifecyclecleanup (#77) and ADR-113",
	},
	{
		Subpath:              "processor/agentic-tools",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "ADR-108, ADR-112, ADR-113, semengine#29, semengine#77",
		ConsumerAction:       "M1 (#402) design; M2 (#404, #287): absorb tool processor + ExecutorRegistry; inject registry directly instead of Dependencies.ToolRegistry",
	},
	{
		Subpath:              "processor/agentic-tools/executors",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "semengine#29",
		ConsumerAction:       "M1 (#402) design; M2 (#404): absorb builtin tool executors used by SemTeams",
	},
	{
		Subpath:              "processor/agentic-tools/runner",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#402) design; M2 (#404): absorb sandbox/command runner used by SemTeams tools",
	},
	{
		Subpath:              "vocabulary/agentic",
		Group:                groupSemTeamsAbsorb,
		SemEngineDisposition: "defer-exclude",
		PostPortHome:         "semteams agent runtime (ADR-061)",
		UpstreamRefs:         "semengine#8 (03B D4), semengine#234",
		ConsumerAction:       "M1 (#401, done): cmd/semteams/main.go calls agvocab.Register() directly; M1 (#402) design; M2 (#404): absorb agentic predicate constants and register via semengine/vocabulary",
	},

	// Group 4: Drop / decouple in Milestone 1 (23 paths: 18 uncomposed factories + 5 helpers/aggregators)
	{
		Subpath:              "componentregistry",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (aggregator cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#3 (03B Q1)",
		ConsumerAction:       "M1 (#399): delete aggregator comparison in internal/runtimecatalog/catalog_test.go",
	},
	{
		Subpath:              "gateway/http",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "defer-exclude",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "gateway/lifecycle-gateway",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (gateway cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "governance",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (governance cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4), semengine#28",
		ConsumerAction:       "M1 (#399): drop unused payload registration from internal/runtimecatalog/payloads.go once gated-dag/agentic-governance leave",
	},
	{
		Subpath:              "input/file",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (I/O cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "input/udp",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (I/O cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "input/websocket",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (outside 04A)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#9",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "output/file",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (I/O cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "output/httppost",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (I/O cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "output/websocket",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (setup-04a-05)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#34",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "payloadbuiltins",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (aggregator cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#3 (03B Q1)",
		ConsumerAction:       "M1 (#399): delete aggregator comparison in internal/runtimecatalog/catalog_test.go",
	},
	{
		Subpath:              "pkg/logging",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (cut)",
		PostPortHome:         "dropped in M1 (#401)",
		UpstreamRefs:         "semengine#8 (tier1-cross-check)",
		ConsumerAction:       "M1 (#401, done): replaced pkg/logging in cmd/semteams/logging.go with stdlib slog.NewMultiHandler and product-owned natsLogHandler",
	},
	{
		Subpath:              "processor/agentic-governance",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (agentic cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4), semengine#28",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "processor/gated-dag",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (aggregator cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#3 (03B Q1)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory + payload registration from internal/runtimecatalog",
	},
	{
		Subpath:              "processor/graph-clustering",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (next before 04a-06)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#9 (2026-11-08 ruling)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog (re-add from SemEngine only if NLQ text search is composed)",
	},
	{
		Subpath:              "processor/graph-embedding",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (setup-04a-05)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#9",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "processor/graph-index",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "adapt (on SemEngine main)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "ADR-108, ADR-109",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog (available in semengine/processor/graph-index if needed later)",
	},
	{
		Subpath:              "processor/graph-index-spatial",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (setup-04a-07)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "ADR-108, semengine#9",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "processor/graph-index-temporal",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (setup-04a-07)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "ADR-108, semengine#9",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "processor/json_filter",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (JSON cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "processor/json_generic",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (JSON cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "processor/json_map",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (JSON cut)",
		PostPortHome:         "dropped in M1 (#399)",
		UpstreamRefs:         "semengine#8 (03B D4)",
		ConsumerAction:       "M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas",
	},
	{
		Subpath:              "vocabulary/builtins",
		Group:                groupMilestone1Prune,
		SemEngineDisposition: "unported (aggregator cut)",
		PostPortHome:         "dropped in M1 (#401)",
		UpstreamRefs:         "semengine#3 (03B Q1), semengine#234",
		ConsumerAction:       "M1 (#401, done): audited wired predicates (0 rulepacks hits) and replaced builtins.Register() in cmd/semteams/main.go with agvocab.Register()",
	},
}

type rowReport struct {
	ImportPath           string   `json:"import_path"`
	Subpath              string   `json:"subpath"`
	Group                string   `json:"group"`
	ProdFiles            int      `json:"non_test_files"`
	TestFiles            int      `json:"test_files"`
	RepoFiles            int      `json:"repo_files"`
	ScriptsImportFiles   int      `json:"scripts_import_files"`
	SemEngineDisposition string   `json:"semengine_disposition"`
	PostPortHome         string   `json:"post_port_home"`
	UpstreamRefs         string   `json:"upstream_refs"`
	ConsumerAction       string   `json:"consumer_action"`
	ProdFilePaths        []string `json:"non_test_file_paths,omitempty"`
	TestFilePaths        []string `json:"test_file_paths,omitempty"`
	ScriptsFilePaths     []string `json:"scripts_file_paths,omitempty"`
}

type groupSummary struct {
	Group          string `json:"group"`
	CatalogPaths   int    `json:"catalog_paths"`
	ActivePaths    int    `json:"active_paths"`
	PrunedPaths    int    `json:"pruned_paths"`
	ProdImports    int    `json:"non_test_import_edges"`
	TestImports    int    `json:"test_import_edges"`
	ScriptsImports int    `json:"scripts_import_edges"`
}

type scriptLiteralHit struct {
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

type ledgerSummary struct {
	Module                     string             `json:"framework_module"`
	CatalogTotalPaths          int                `json:"catalog_total_paths"`
	ActiveRepoImportPaths      int                `json:"active_repo_import_paths"`
	PrunedCatalogPaths         int                `json:"pruned_catalog_paths"`
	DistinctProdFilesImporting int                `json:"distinct_non_test_files_importing"`
	DistinctTestFilesImporting int                `json:"distinct_test_files_importing"`
	DistinctScriptsImporting   int                `json:"distinct_scripts_files_importing"`
	ScriptsLiteralOnlyPaths    []scriptLiteralHit `json:"scripts_literal_only_paths"`
	TotalStringMatchedPaths    int                `json:"total_string_matched_paths"`
	GroupSummaries             []groupSummary     `json:"group_summaries"`
	Rows                       []rowReport        `json:"rows"`
}

var semstreamsLiteralPattern = regexp.MustCompile(`github\.com/c360studio/semstreams(?:/[A-Za-z0-9_./-]+)?`)

func main() {
	rootFlag := flag.String("root", ".", "SemTeams repository root directory to scan")
	formatFlag := flag.String("format", "markdown", "output format: markdown or json")
	activeOnlyFlag := flag.Bool("active-only", false, "include only paths with >=1 remaining import in rows")
	flag.Parse()

	if *formatFlag != "markdown" && *formatFlag != "json" {
		fatalf("unsupported -format %q (want markdown or json)", *formatFlag)
	}

	absRoot, err := filepath.Abs(*rootFlag)
	if err != nil {
		fatalf("resolve -root: %v", err)
	}

	summary, err := measure(absRoot, *activeOnlyFlag)
	if err != nil {
		fatalf("%v", err)
	}

	if *formatFlag == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			fatalf("encode json: %v", err)
		}
		return
	}

	os.Stdout.WriteString(renderMarkdown(summary))
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "measure-semengine-port-ledger: "+format+"\n", args...)
	os.Exit(1)
}

type pathAccumulator struct {
	prod    map[string]bool
	test    map[string]bool
	scripts map[string]bool
}

func measure(root string, activeOnly bool) (ledgerSummary, error) {
	catalogBySubpath := make(map[string]catalogEntry, len(ledgerCatalog))
	for _, entry := range ledgerCatalog {
		if _, dup := catalogBySubpath[entry.Subpath]; dup {
			return ledgerSummary{}, fmt.Errorf("duplicate catalog subpath %q", entry.Subpath)
		}
		catalogBySubpath[entry.Subpath] = entry
	}

	acc := make(map[string]*pathAccumulator, len(ledgerCatalog))
	getAcc := func(sub string) *pathAccumulator {
		a := acc[sub]
		if a == nil {
			a = &pathAccumulator{
				prod:    map[string]bool{},
				test:    map[string]bool{},
				scripts: map[string]bool{},
			}
			acc[sub] = a
		}
		return a
	}

	distinctProd := map[string]bool{}
	distinctTest := map[string]bool{}
	distinctScripts := map[string]bool{}
	scriptLiterals := map[string]map[string]bool{}
	var unclassified []string

	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".svelte-kit", ".worktrees":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		inScripts := strings.HasPrefix(rel, "scripts/")
		isTest := strings.HasSuffix(rel, "_test.go")

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileAST, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}

		importLitPos := make(map[token.Pos]bool, len(fileAST.Imports))
		for _, imp := range fileAST.Imports {
			importLitPos[imp.Path.Pos()] = true
			impPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", rel, err)
			}
			if impPath != semstreamsModule && !strings.HasPrefix(impPath, semstreamsModule+"/") {
				continue
			}
			sub := strings.TrimPrefix(impPath, semstreamsModule+"/")
			if impPath == semstreamsModule {
				sub = ""
			}
			if _, ok := catalogBySubpath[sub]; !ok {
				unclassified = append(unclassified, fmt.Sprintf("%s (%s)", impPath, rel))
				continue
			}
			rec := getAcc(sub)
			switch {
			case inScripts:
				rec.scripts[rel] = true
				distinctScripts[rel] = true
			case isTest:
				rec.test[rel] = true
				distinctTest[rel] = true
			default:
				rec.prod[rel] = true
				distinctProd[rel] = true
			}
		}

		if inScripts {
			ast.Inspect(fileAST, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING || importLitPos[lit.Pos()] {
					return true
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				for _, hit := range semstreamsLiteralPattern.FindAllString(val, -1) {
					if scriptLiterals[hit] == nil {
						scriptLiterals[hit] = map[string]bool{}
					}
					scriptLiterals[hit][rel] = true
				}
				return true
			})
		}
		return nil
	})
	if walkErr != nil {
		return ledgerSummary{}, walkErr
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		return ledgerSummary{}, fmt.Errorf("unclassified SemStreams import(s): %s", strings.Join(unclassified, ", "))
	}

	groupOrder := []string{
		groupSemEngineMain,
		groupSemEngine04A06,
		groupSemTeamsAbsorb,
		groupMilestone1Prune,
	}
	groupMap := make(map[string]*groupSummary, len(groupOrder))
	for _, g := range groupOrder {
		groupMap[g] = &groupSummary{Group: g}
	}

	activeASTPaths := map[string]bool{}
	var rows []rowReport
	activeRepoPaths := 0
	prunedPaths := 0

	for _, entry := range ledgerCatalog {
		rec := getAcc(entry.Subpath)
		prodList := sortedKeys(rec.prod)
		testList := sortedKeys(rec.test)
		scriptsList := sortedKeys(rec.scripts)
		repoCount := len(prodList) + len(testList)
		fullImport := semstreamsModule + "/" + entry.Subpath

		gs := groupMap[entry.Group]
		gs.CatalogPaths++
		gs.ProdImports += len(prodList)
		gs.TestImports += len(testList)
		gs.ScriptsImports += len(scriptsList)

		if repoCount > 0 {
			activeRepoPaths++
			gs.ActivePaths++
		} else {
			prunedPaths++
			gs.PrunedPaths++
		}
		if repoCount > 0 || len(scriptsList) > 0 {
			activeASTPaths[fullImport] = true
		}
		if activeOnly && repoCount == 0 && len(scriptsList) == 0 {
			continue
		}

		rows = append(rows, rowReport{
			ImportPath:           fullImport,
			Subpath:              entry.Subpath,
			Group:                entry.Group,
			ProdFiles:            len(prodList),
			TestFiles:            len(testList),
			RepoFiles:            repoCount,
			ScriptsImportFiles:   len(scriptsList),
			SemEngineDisposition: entry.SemEngineDisposition,
			PostPortHome:         entry.PostPortHome,
			UpstreamRefs:         entry.UpstreamRefs,
			ConsumerAction:       entry.ConsumerAction,
			ProdFilePaths:        prodList,
			TestFilePaths:        testList,
			ScriptsFilePaths:     scriptsList,
		})
	}

	var literalOnly []scriptLiteralHit
	for _, litPath := range sortedMapKeys(scriptLiterals) {
		if activeASTPaths[litPath] {
			continue
		}
		literalOnly = append(literalOnly, scriptLiteralHit{
			Path:  litPath,
			Files: sortedKeys(scriptLiterals[litPath]),
		})
	}

	var groups []groupSummary
	for _, g := range groupOrder {
		groups = append(groups, *groupMap[g])
	}

	return ledgerSummary{
		Module:                     semstreamsModule,
		CatalogTotalPaths:          len(ledgerCatalog),
		ActiveRepoImportPaths:      activeRepoPaths,
		PrunedCatalogPaths:         prunedPaths,
		DistinctProdFilesImporting: len(distinctProd),
		DistinctTestFilesImporting: len(distinctTest),
		DistinctScriptsImporting:   len(distinctScripts),
		ScriptsLiteralOnlyPaths:    literalOnly,
		TotalStringMatchedPaths:    len(activeASTPaths) + len(literalOnly),
		GroupSummaries:             groups,
		Rows:                       rows,
	}, nil
}

func renderMarkdown(s ledgerSummary) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "### Summary counts\n\n")
	fmt.Fprintf(&b, "- Active repository `.go` import paths (excluding `scripts/`): **%d** of **%d** baseline paths (**%d** pruned)\n",
		s.ActiveRepoImportPaths, s.CatalogTotalPaths, s.PrunedCatalogPaths)
	fmt.Fprintf(&b, "- Distinct importing files: **%d** non-test `.go`, **%d** `_test.go`, **%d** `scripts/*.go`\n",
		s.DistinctProdFilesImporting, s.DistinctTestFilesImporting, s.DistinctScriptsImporting)
	fmt.Fprintf(&b, "- `scripts/` string-literal-only SemStreams paths: **%d** (total string-matched paths across repo + `scripts/`: **%d**)\n\n",
		len(s.ScriptsLiteralOnlyPaths), s.TotalStringMatchedPaths)

	fmt.Fprintf(&b, "| Group | Baseline paths | Active paths | Pruned paths | Non-test file imports | `_test.go` file imports | `scripts/` imports |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, g := range s.GroupSummaries {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d |\n",
			g.Group, g.CatalogPaths, g.ActivePaths, g.PrunedPaths, g.ProdImports, g.TestImports, g.ScriptsImports)
	}

	for _, g := range s.GroupSummaries {
		var groupRows []rowReport
		for _, r := range s.Rows {
			if r.Group == g.Group {
				groupRows = append(groupRows, r)
			}
		}
		if len(groupRows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n\n", g.Group)
		fmt.Fprintf(&b, "| SemStreams subpath | Non-test | `_test.go` | `scripts/` | SemEngine disposition | Post-port home | Upstream refs | SemTeams action |\n")
		fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | --- | --- | --- | --- |\n")
		for _, r := range groupRows {
			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | `%s` | `%s` | %s | %s |\n",
				r.Subpath, r.ProdFiles, r.TestFiles, r.ScriptsImportFiles,
				r.SemEngineDisposition, r.PostPortHome, r.UpstreamRefs, r.ConsumerAction)
		}
	}

	if len(s.ScriptsLiteralOnlyPaths) > 0 {
		fmt.Fprintf(&b, "\n### `scripts/` string-literal-only occurrences\n\n")
		fmt.Fprintf(&b, "| Literal string | `scripts/` files |\n")
		fmt.Fprintf(&b, "| --- | --- |\n")
		for _, hit := range s.ScriptsLiteralOnlyPaths {
			fmt.Fprintf(&b, "| `%s` | `%s` |\n", hit.Path, strings.Join(hit.Files, "`, `"))
		}
	}
	return b.String()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
