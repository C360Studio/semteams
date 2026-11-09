package contract

import (
	"os"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/composition"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semteams/internal/runtimecatalog"
)

// These are the two shipped compositions. The same declaration-based validator
// gates boot, so an upstream port change is caught without starting production.
func TestShippedBootstrapCompositions(t *testing.T) {
	registry := component.NewRegistry()
	if err := runtimecatalog.RegisterComponents(registry); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.NewLoader().LoadFromBytes(data)
			if err != nil {
				t.Fatal(err)
			}
			composition.AssertValid(t, registry, cfg)
		})
	}
}

func inspectBootstrapComposition(configPath string) (string, int, []baselineCompositionWarning, int, error) {
	registry := component.NewRegistry()
	if err := runtimecatalog.RegisterComponents(registry); err != nil {
		return "", 0, nil, 0, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", 0, nil, 0, err
	}
	cfg, err := config.NewLoader().LoadFromBytes(data)
	if err != nil {
		return "", 0, nil, 0, err
	}
	res, err := composition.Validate(registry, cfg)
	if err != nil {
		return "", 0, nil, 0, err
	}
	warnings := make([]baselineCompositionWarning, 0, len(res.Warnings))
	for _, w := range res.Warnings {
		warnings = append(warnings, baselineCompositionWarning{
			Type:      w.Type,
			Severity:  w.Severity,
			Component: w.Component,
			Port:      w.Port,
			Message:   w.Message,
		})
	}
	return res.Status, len(res.Errors), warnings, len(composition.Catalog(registry)), nil
}
