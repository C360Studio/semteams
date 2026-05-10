package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRulesFilesPathsResolveOnDisk asserts that every path in every config's
// rule-processor `rules_files` list resolves to a file on disk relative to
// the repo root. Boots fail when any path is missing (upstream rule loader
// returns an error from os.ReadFile and the rule manager refuses to start).
//
// Motivating regression: ADR-040 PR 3 deleted two rule files but missed
// the rules_files references in osh-demo.json + e2e-dev-via-spec.json +
// e2e-research-mode-transition.json. Caught in go-reviewer pass; this test
// prevents the regression class for all future rule renames/deletes.
//
// The test reads each top-level configs/*.json file, walks the components
// map for any component whose config carries a rules_files array, and
// asserts each path resolves. Paths in configs are container-absolute
// ("/app/configs/...") because configs are mounted into the backend
// container at /app — strip the /app/ prefix to get a repo-relative path.
func TestRulesFilesPathsResolveOnDisk(t *testing.T) {
	configs, err := filepath.Glob("../../configs/*.json")
	if err != nil {
		t.Fatalf("glob configs: %v", err)
	}
	if len(configs) == 0 {
		t.Fatal("no configs found — wrong working directory?")
	}

	for _, cfgPath := range configs {
		t.Run(filepath.Base(cfgPath), func(t *testing.T) {
			data, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatalf("read %s: %v", cfgPath, err)
			}

			var cfg struct {
				Components map[string]struct {
					Config json.RawMessage `json:"config"`
				} `json:"components"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("unmarshal %s: %v", cfgPath, err)
			}

			for compName, comp := range cfg.Components {
				if len(comp.Config) == 0 {
					continue
				}
				var compCfg struct {
					RulesFiles []string `json:"rules_files"`
				}
				if err := json.Unmarshal(comp.Config, &compCfg); err != nil {
					// Component config doesn't fit this shape; skip.
					continue
				}
				if len(compCfg.RulesFiles) == 0 {
					continue
				}

				for _, rulePath := range compCfg.RulesFiles {
					// Container-absolute paths look like "/app/configs/...".
					// Strip the /app/ prefix to get a repo-relative path.
					rel := strings.TrimPrefix(rulePath, "/app/")
					if rel == rulePath {
						t.Errorf("%s component %s: rules_files entry %q does not start with /app/ — operator-mounted path convention violated",
							filepath.Base(cfgPath), compName, rulePath)
						continue
					}
					hostPath := filepath.Join("../..", rel)
					if _, err := os.Stat(hostPath); err != nil {
						t.Errorf("%s component %s: rules_files entry %q does not resolve on disk — boot would fail (host path %s: %v)",
							filepath.Base(cfgPath), compName, rulePath, hostPath, err)
					}
				}
			}
		})
	}
}
