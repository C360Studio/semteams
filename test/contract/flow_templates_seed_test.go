package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/c360studio/semstreams/flowtemplate"
	"github.com/stretchr/testify/require"
)

// kebabCaseFlowID pins the rendered flow ID shape. Catches the class of
// bug where a template substitutes a human-readable parameter (e.g.
// "{{.topic}}" = "Foo Bar") into the flow ID slot instead of its
// paired slug variant ("{{.topic_slug}}" = "foo-bar"). See README §"A
// note on slug parameters." Empty IDs already fail Flow.Validate.
var kebabCaseFlowID = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// TestFlowTemplatesSeedConsistency walks configs/flow-templates/*.json and
// asserts every file:
//   - parses as flowtemplate.Template
//   - passes Template.Validate()
//   - Instantiate() with declared defaults produces a non-nil flow with a
//     non-empty ID after substitution
//   - the rendered flow passes flowstore.Flow.Validate() (ADR-042 Phase 2a)
//
// ADR-042 Phase 1+2a — guards the seed inventory against malformed
// templates that would silently drop on boot under the loader's
// warn-and-skip-on-validation-failure policy. A template that fails this
// test would deploy as missing-from-inventory in production.
//
// Empty inventory (no *.json files) is intentional during Phase 1; Phase
// 2a populates the first three (research-pipeline, dev-via-spec-pipeline,
// web-research) as minimal flow skeletons. Phase 2b expands the body
// to runnable agentic topologies.
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
			require.NotEmpty(t, flow.Name, "instantiated flow must have a name after substitution")

			// Phase 2a strengthening: the rendered flow MUST pass
			// flowstore.Flow.Validate(). Skeleton-only bodies still
			// satisfy this (id + name + valid runtime_state + empty
			// nodes/connections); Phase 2b's expanded bodies will
			// satisfy it via real node/connection structure.
			require.NoError(t, flow.Validate(),
				"rendered flow must pass Flow.Validate() — caught a malformed body string before it reaches create_flow")

			// Flow ID must be kebab-case. Pins the slug-parameter
			// convention: if a template substitutes a human-readable
			// parameter into the ID slot (e.g. "{{.topic}}" instead of
			// "{{.topic_slug}}"), the rendered ID will have whitespace
			// or uppercase and this assertion fails loud. Empty IDs
			// fail Flow.Validate above, so we only see non-empty here.
			require.Regexp(t, kebabCaseFlowID, flow.ID,
				"rendered flow ID must be kebab-case (use the paired _slug parameter, not the human-readable one)")
		})
	}
}
