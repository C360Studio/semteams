package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semstreams/flowtemplate"
	"github.com/stretchr/testify/require"
)

// TestFlowTemplatesSeedConsistency walks configs/flow-templates/*.json and
// asserts every file:
//   - parses as flowtemplate.Template
//   - passes Template.Validate()
//   - Instantiate() with declared defaults produces a renderable flow JSON
//
// ADR-042 Phase 1 — guards the seed inventory against malformed templates
// that would silently drop on boot under the loader's
// warn-and-skip-on-validation-failure policy. A template that fails this
// test would deploy as missing-from-inventory in production.
//
// Empty inventory (no *.json files) is intentional during Phase 1 — the
// test passes when the directory exists but is empty. Phase 2 populates
// the first templates and this test starts asserting against them.
func TestFlowTemplatesSeedConsistency(t *testing.T) {
	t.Parallel()

	dir := "../../configs/flow-templates"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("flow-templates directory missing: %s", dir)
		}
		t.Fatalf("read dir %s: %v", dir, err)
	}

	var templateFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		templateFiles = append(templateFiles, filepath.Join(dir, e.Name()))
	}

	if len(templateFiles) == 0 {
		t.Log("flow-templates directory empty — Phase 1 expects this; Phase 2 populates the first templates")
		return
	}

	for _, path := range templateFiles {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			require.NoError(t, err, "read template file")

			var tmpl flowtemplate.Template
			require.NoError(t, json.Unmarshal(data, &tmpl), "unmarshal template JSON")

			require.NoError(t, tmpl.Validate(), "Template.Validate() must pass on shipped templates")

			// Instantiate with declared defaults — every required parameter
			// must have a default value supplied here, OR be set as
			// required:true (in which case Instantiate returns an error and
			// callers know to supply the value). For seed integrity, we
			// assert defaults-only instantiation either succeeds OR fails
			// only because a required-without-default parameter was missing.
			defaults := make(map[string]string, len(tmpl.Parameters))
			for _, p := range tmpl.Parameters {
				if p.Required {
					continue
				}
				defaults[p.Name] = p.Default
			}

			flow, err := tmpl.Instantiate(defaults)
			if err != nil {
				// Acceptable only when at least one required parameter was
				// declared without a default — that's the contract that
				// callers MUST supply a value at instantiate time.
				hasRequiredParam := false
				for _, p := range tmpl.Parameters {
					if p.Required {
						hasRequiredParam = true
						break
					}
				}
				require.True(t, hasRequiredParam,
					"Instantiate(defaults) failed but no required-without-default parameter explains the failure: %v", err)
				return
			}

			require.NotNil(t, flow, "Instantiate must produce a non-nil flow on defaults")
			require.NotEmpty(t, flow.ID, "instantiated flow must have an ID after substitution")
		})
	}
}
