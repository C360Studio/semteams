package contract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/testharness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperatorTestHarnessCatalogParses verifies the operator-shipped
// test harness catalogs in configs/harnesses*.json all parse + validate
// against the schema in cmd/semteams/testharness/catalog.go. Without
// this contract pin, a typo in the JSON would only surface at
// container boot — and the boot path tolerates a missing file
// (catalog stays empty, see flags.go:44 SEMTEAMS_HARNESS_CATALOG_PATH
// help text), so a malformed file would silently render the
// "no test harnesses" fragment instead of failing fast.
//
// Every file matching the glob is parsed; named-entry assertions
// keep the meshtasticd-3.x catalog (R3.7.2.e′) honest as fields
// evolve. Add per-test-harness assertions here when new entries land,
// not at the catalog.go schema layer (which stays structural-only).
func TestOperatorTestHarnessCatalogParses(t *testing.T) {
	files, err := filepath.Glob("../../configs/harnesses*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files, "no configs/harnesses*.json files found — wrong working directory?")

	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			entries, err := testharness.ParseFile(data)
			require.NoError(t, err, "%s failed schema parse/validate", name)
			t.Logf("%s parsed %d entries", name, len(entries))
		})
	}
}

// TestMeshtasticdEntryShape pins the canonical fields of the
// meshtasticd-3.x catalog entry so a careless edit (typo in
// smoke_contract_schema, regression to compose_profile usage) gets
// caught at test time. ADR-034 made compose_profile optional for
// process-local-testcontainer flows; this entry should stay free of
// it as the lifecycle is managed by Testcontainers under DooD.
func TestMeshtasticdEntryShape(t *testing.T) {
	data, err := os.ReadFile("../../configs/harnesses.json")
	require.NoError(t, err)

	entries, err := testharness.ParseFile(data)
	require.NoError(t, err)

	var got *testharness.TestHarness
	for i := range entries {
		if entries[i].Name == "meshtasticd-3.x" {
			got = &entries[i]
			break
		}
	}
	require.NotNil(t, got, "meshtasticd-3.x entry missing from configs/harnesses.json")

	assert.Empty(t, got.ComposeProfile,
		"meshtasticd-3.x must NOT set compose_profile — process-local-testcontainer runtime manages lifecycle in-process via Testcontainers under sandbox DooD (ADR-034)")
	// Image tag is loosened on purpose: the upstream image will rev
	// (3.5.0 → 3.6.0 → ...) faster than this contract test's review
	// cadence. Pin the repository so a typo regression
	// (`meshtasticc/meshtasticd`, etc.) still fails loudly; let the
	// version float so a routine bump doesn't masquerade as a contract
	// break. Schema name + protobuf port stay exact below — those are
	// protocol-stable.
	assert.Contains(t, got.Image, "meshtastic/meshtasticd:",
		"image must reference the upstream meshtastic/meshtasticd repo (any 3.x tag is fine)")
	assert.Equal(t, "meshtasticd.smoke_contract.v1", got.SmokeContractSchema,
		"smoke_contract_schema name must match the daemon's `meshtasticd-`-prefixed harness name; ADR-033's earlier `meshtastic.smoke_contract.v1` was inconsistent and was corrected here")
	require.Len(t, got.Exposes.TCP, 1, "expect exactly one TCP exposure (the protobuf API on :4403)")
	assert.Equal(t, 4403, got.Exposes.TCP[0].Port)
	assert.Equal(t, "meshtastic-protobuf", got.Exposes.TCP[0].Protocol)
	assert.NotEmpty(t, got.DomainDescription, "renderer prints this verbatim into the researcher persona fragment")
}
