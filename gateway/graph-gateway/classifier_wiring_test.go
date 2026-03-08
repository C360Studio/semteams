package graphgateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================================
// Classifier wiring tests
//
// These tests verify that CreateGraphGateway builds the right ClassifierChain
// based on the EnableEmbeddingClassifier flag and DomainExamplesPath config.
//
// Tier detection is confirmed behaviourally: when an embedding classifier is
// present, queries that have no keyword-level intent but match a domain example
// should return Tier 1 results.  When only T0 is loaded, such queries return
// the default T0 result.
// ====================================================================================

// newTestNATSClient creates an unconnected NATS client usable in unit tests.
func newTestNATSClient(t *testing.T) *natsclient.Client {
	t.Helper()
	nc, err := natsclient.NewClient("nats://localhost:4222")
	require.NoError(t, err)
	return nc
}

// createTestDomainDir writes minimal valid domain JSON files to a temp directory
// and returns the directory path.  The directory is removed when the test ends.
func createTestDomainDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	domains := []struct {
		filename string
		content  string
	}{
		{
			"iot.json",
			`{
				"domain": "iot",
				"version": "1.0",
				"examples": [
					{"query": "Show sensor SENS-001 status", "intent": "entity_lookup"},
					{"query": "Find gateway GW-MAIN", "intent": "entity_lookup"},
					{"query": "List sensors offline since Tuesday", "intent": "temporal_filter"},
					{"query": "How many sensors are reporting?", "intent": "aggregation"},
					{"query": "Count devices in error state", "intent": "aggregation"}
				]
			}`,
		},
		{
			"robotics.json",
			`{
				"domain": "robotics",
				"version": "1.0",
				"examples": [
					{"query": "Show robot arm position", "intent": "entity_lookup"},
					{"query": "What tasks did robot complete today?", "intent": "temporal_filter"}
				]
			}`,
		},
	}

	for _, d := range domains {
		path := filepath.Join(dir, d.filename)
		require.NoError(t, os.WriteFile(path, []byte(d.content), 0o644))
	}

	return dir
}

// createGatewayFromConfig marshals cfg to JSON and calls CreateGraphGateway.
func createGatewayFromConfig(t *testing.T, cfg Config) *Component {
	t.Helper()

	configJSON, err := json.Marshal(cfg)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: newTestNATSClient(t),
	}

	comp, err := CreateGraphGateway(configJSON, deps)
	require.NoError(t, err)
	require.NotNil(t, comp)

	return comp.(*Component)
}

// classifyWith runs a query through comp.classifier and returns the result.
func classifyWith(t *testing.T, comp *Component, q string) *query.ClassificationResult {
	t.Helper()
	result := comp.classifier.ClassifyQuery(context.Background(), q)
	require.NotNil(t, result, "ClassifyQuery must return a non-nil result")
	return result
}

// ====================================================================================
// T0-only — embedding disabled
// ====================================================================================

func TestClassifierChain_T0Only_WhenEmbeddingDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = false
	cfg.DomainExamplesPath = ""

	comp := createGatewayFromConfig(t, cfg)

	// Classifier chain itself must always exist.
	assert.NotNil(t, comp.classifier, "ClassifierChain must be initialised")

	// An ambiguous query with no keyword cues must yield Tier 0.
	result := classifyWith(t, comp, "How many sensors are reporting?")
	assert.Equal(t, 0, result.Tier, "T0-only chain must return Tier 0 for non-keyword queries")
}

func TestClassifierChain_T0Only_WhenFlagFalseButPathProvided(t *testing.T) {
	// Flag takes precedence even when a valid path is given.
	domainDir := createTestDomainDir(t)

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = false
	cfg.DomainExamplesPath = domainDir

	comp := createGatewayFromConfig(t, cfg)

	result := classifyWith(t, comp, "How many sensors are reporting?")
	assert.Equal(t, 0, result.Tier,
		"embedding classifier must not be loaded when the flag is false")
}

// ====================================================================================
// T0+T1/T2 chain — embedding enabled with valid examples
// ====================================================================================

func TestClassifierChain_T1Enabled_WithValidDirectory(t *testing.T) {
	domainDir := createTestDomainDir(t)

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = domainDir

	comp := createGatewayFromConfig(t, cfg)
	assert.NotNil(t, comp.classifier)

	// A query that semantically matches an example (but has no keyword-level cues)
	// should route through the embedding tier and return Tier >= 1.
	// "Show sensor SENS-001 status" matches the "entity_lookup" intent example
	// and has no temporal/spatial/path/similarity/aggregation keyword patterns.
	result := classifyWith(t, comp, "Show sensor SENS-001 status")
	assert.GreaterOrEqual(t, result.Tier, 1,
		"embedding classifier should elevate ambiguous domain queries to Tier 1+")
}

func TestClassifierChain_T1Enabled_WithSingleFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "iot.json")
	content := `{
		"domain": "iot",
		"version": "1.0",
		"examples": [
			{"query": "Show sensor status", "intent": "entity_lookup"},
			{"query": "How many sensors are reporting?", "intent": "aggregation"},
			{"query": "Count devices in error state", "intent": "aggregation"}
		]
	}`
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = filePath

	comp := createGatewayFromConfig(t, cfg)
	assert.NotNil(t, comp.classifier)

	result := classifyWith(t, comp, "Show sensor status")
	assert.GreaterOrEqual(t, result.Tier, 1,
		"single-file domain examples should enable embedding classifier")
}

// ====================================================================================
// Graceful fallback — embedding enabled but path is missing/empty
// ====================================================================================

func TestClassifierChain_FallbackToT0_WhenPathDoesNotExist(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = "/nonexistent/path/that/does/not/exist"

	// Factory must succeed; a missing path is non-fatal.
	comp := createGatewayFromConfig(t, cfg)
	assert.NotNil(t, comp.classifier)

	result := classifyWith(t, comp, "How many sensors are reporting?")
	assert.Equal(t, 0, result.Tier,
		"missing path must fall back to T0-only classification")
}

func TestClassifierChain_FallbackToT0_WhenPathIsEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = ""

	comp := createGatewayFromConfig(t, cfg)
	assert.NotNil(t, comp.classifier)

	result := classifyWith(t, comp, "How many sensors are reporting?")
	assert.Equal(t, 0, result.Tier, "empty path must fall back to T0-only classification")
}

func TestClassifierChain_FallbackToT0_WhenDirectoryIsEmpty(t *testing.T) {
	emptyDir := t.TempDir()

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = emptyDir

	comp := createGatewayFromConfig(t, cfg)
	assert.NotNil(t, comp.classifier)

	result := classifyWith(t, comp, "How many sensors are reporting?")
	assert.Equal(t, 0, result.Tier,
		"empty directory must fall back to T0-only classification")
}

func TestClassifierChain_FallbackToT0_WhenFileIsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{not valid json`), 0o644))

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = bad

	comp := createGatewayFromConfig(t, cfg)
	assert.NotNil(t, comp.classifier)

	result := classifyWith(t, comp, "How many sensors are reporting?")
	assert.Equal(t, 0, result.Tier,
		"invalid JSON file must fall back to T0-only classification")
}

func TestClassifierChain_PartialLoad_SkipsBadFiles(t *testing.T) {
	// A directory with one good file and one bad file should still enable embedding.
	dir := t.TempDir()

	good := filepath.Join(dir, "iot.json")
	goodContent := `{
		"domain": "iot", "version": "1.0",
		"examples": [
			{"query": "How many sensors are reporting?", "intent": "aggregation"},
			{"query": "Count devices in error state", "intent": "aggregation"},
			{"query": "Show sensor status", "intent": "entity_lookup"}
		]
	}`
	require.NoError(t, os.WriteFile(good, []byte(goodContent), 0o644))

	bad := filepath.Join(dir, "broken.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{broken`), 0o644))

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = dir

	comp := createGatewayFromConfig(t, cfg)
	assert.NotNil(t, comp.classifier)

	// Good file loaded → embedding classifier active → Tier 1+ for domain queries.
	result := classifyWith(t, comp, "Show sensor status")
	assert.GreaterOrEqual(t, result.Tier, 1,
		"partial load should still enable embedding when at least one file is valid")
}

// ====================================================================================
// Config field defaults and JSON round-trip
// ====================================================================================

func TestConfig_NewFields_DefaultToFalseAndEmpty(t *testing.T) {
	cfg := DefaultConfig()

	assert.False(t, cfg.EnableEmbeddingClassifier,
		"EnableEmbeddingClassifier must default to false")
	assert.Empty(t, cfg.DomainExamplesPath,
		"DomainExamplesPath must default to empty string")
}

func TestConfig_NewFields_SurviveJSONRoundTrip(t *testing.T) {
	original := DefaultConfig()
	original.EnableEmbeddingClassifier = true
	original.DomainExamplesPath = "/configs/domains"

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored Config
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, original.EnableEmbeddingClassifier, restored.EnableEmbeddingClassifier)
	assert.Equal(t, original.DomainExamplesPath, restored.DomainExamplesPath)
}

// ====================================================================================
// loadEmbeddingClassifier unit tests (package-level function, same package)
// ====================================================================================

func TestLoadEmbeddingClassifier_ReturnNil_WhenDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = false

	result := loadEmbeddingClassifier(cfg, slog.Default())

	assert.Nil(t, result, "must return nil when embedding classifier is disabled")
}

func TestLoadEmbeddingClassifier_ReturnNil_WhenPathEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = ""

	result := loadEmbeddingClassifier(cfg, slog.Default())

	assert.Nil(t, result, "must return nil when path is empty")
}

func TestLoadEmbeddingClassifier_ReturnNil_WhenPathNotExist(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = "/does/not/exist"

	result := loadEmbeddingClassifier(cfg, slog.Default())

	assert.Nil(t, result, "must return nil when path does not exist")
}

func TestLoadEmbeddingClassifier_ReturnNil_WhenDirEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = t.TempDir()

	result := loadEmbeddingClassifier(cfg, slog.Default())

	assert.Nil(t, result, "must return nil when directory has no JSON files")
}

func TestLoadEmbeddingClassifier_ReturnClassifier_WithValidDir(t *testing.T) {
	domainDir := createTestDomainDir(t)

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = domainDir

	result := loadEmbeddingClassifier(cfg, slog.Default())

	assert.NotNil(t, result, "must return a classifier when examples are available")
}

func TestLoadEmbeddingClassifier_ReturnClassifier_WithSingleFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.json")
	require.NoError(t, os.WriteFile(fp,
		[]byte(`{"domain":"test","version":"1.0","examples":[{"query":"q","intent":"i"}]}`),
		0o644))

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = fp

	result := loadEmbeddingClassifier(cfg, slog.Default())

	assert.NotNil(t, result, "must return a classifier when a single valid file is provided")
}

func TestLoadEmbeddingClassifier_ReturnClassifier_SkipsInvalidFile(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.json")
	require.NoError(t, os.WriteFile(good,
		[]byte(`{"domain":"iot","version":"1.0","examples":[{"query":"sensor status","intent":"entity_lookup"}]}`),
		0o644))

	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{bad json`), 0o644))

	cfg := DefaultConfig()
	cfg.EnableEmbeddingClassifier = true
	cfg.DomainExamplesPath = dir

	result := loadEmbeddingClassifier(cfg, slog.Default())

	assert.NotNil(t, result, "must return classifier even when some files fail to parse")
}
