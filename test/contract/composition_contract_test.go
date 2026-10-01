package contract

import (
	"os"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/componentregistry"
	"github.com/c360studio/semstreams/composition"
	"github.com/c360studio/semstreams/config"
)

// These are the two shipped compositions. The same declaration-based validator
// gates boot, so an upstream port change is caught without starting production.
func TestShippedBootstrapCompositions(t *testing.T) {
	registry := component.NewRegistry()
	if err := componentregistry.Register(registry); err != nil {
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
