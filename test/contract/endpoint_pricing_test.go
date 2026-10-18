package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/model"
)

var pricingDateRe = regexp.MustCompile(`\b20\d{2}-\d{2}-\d{2}\b`)

type expectedPrice struct {
	in  float64
	out float64
}

// TestBootstrapEndpointsDeclarePricing locks in #248 / #301 Slice 1:
// every model_registry endpoint in configs/flow-bootstrap.json and
// configs/e2e-flow-bootstrap.json must declare positive input and output
// prices per 1M tokens with a dated _comment, and every capability in
// model_registry.capabilities must resolve via model.ResolveEndpointName
// to a priced endpoint so processor/agentic-loop stamps agent.loop.cost-usd
// on both direct-endpoint and capability-spawned loops.
func TestBootstrapEndpointsDeclarePricing(t *testing.T) {
	cases := []struct {
		path   string
		prices map[string]expectedPrice
	}{
		{
			path: "../../configs/flow-bootstrap.json",
			prices: map[string]expectedPrice{
				"claude-haiku":  {in: 1.0, out: 5.0},
				"claude-sonnet": {in: 3.0, out: 15.0},
				"gemini-flash":  {in: 0.5, out: 3.0},
				"gemini-pro":    {in: 2.0, out: 12.0},
			},
		},
		{
			path: "../../configs/e2e-flow-bootstrap.json",
			prices: map[string]expectedPrice{
				"general": {in: 0.5, out: 3.0},
				"fast":    {in: 0.5, out: 3.0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			if errs := validateBootstrapEndpointPricing(data, tc.prices); len(errs) > 0 {
				t.Fatalf("%s endpoint pricing violations:\n- %s", tc.path, strings.Join(errs, "\n- "))
			}
		})
	}
}

// TestBootstrapEndpointsDeclarePricingSensitivity is the paired sensitivity
// test proving validateBootstrapEndpointPricing fails when an endpoint omits
// input/output pricing, omits a dated _comment, or drifts from expected rates.
func TestBootstrapEndpointsDeclarePricingSensitivity(t *testing.T) {
	baseData, err := os.ReadFile("../../configs/flow-bootstrap.json")
	if err != nil {
		t.Fatalf("read flow-bootstrap.json: %v", err)
	}
	wantPrices := map[string]expectedPrice{
		"claude-haiku":  {in: 1.0, out: 5.0},
		"claude-sonnet": {in: 3.0, out: 15.0},
		"gemini-flash":  {in: 0.5, out: 3.0},
		"gemini-pro":    {in: 2.0, out: 12.0},
	}

	mutateEndpoint := func(t *testing.T, endpoint string, fn func(ep map[string]any)) []byte {
		t.Helper()
		var root map[string]any
		if err := json.Unmarshal(baseData, &root); err != nil {
			t.Fatalf("unmarshal base config: %v", err)
		}
		mr := root["model_registry"].(map[string]any)
		eps := mr["endpoints"].(map[string]any)
		ep := eps[endpoint].(map[string]any)
		fn(ep)
		out, err := json.Marshal(root)
		if err != nil {
			t.Fatalf("marshal mutated config: %v", err)
		}
		return out
	}

	t.Run("missing input_price_per_1m_tokens fails", func(t *testing.T) {
		mutated := mutateEndpoint(t, "claude-haiku", func(ep map[string]any) {
			delete(ep, "input_price_per_1m_tokens")
		})
		errs := validateBootstrapEndpointPricing(mutated, wantPrices)
		if !containsSubstring(errs, `endpoint "claude-haiku": input_price_per_1m_tokens must be > 0`) {
			t.Fatalf("expected missing input price error, got: %v", errs)
		}
	})

	t.Run("missing output_price_per_1m_tokens fails", func(t *testing.T) {
		mutated := mutateEndpoint(t, "gemini-flash", func(ep map[string]any) {
			delete(ep, "output_price_per_1m_tokens")
		})
		errs := validateBootstrapEndpointPricing(mutated, wantPrices)
		if !containsSubstring(errs, `endpoint "gemini-flash": output_price_per_1m_tokens must be > 0`) {
			t.Fatalf("expected missing output price error, got: %v", errs)
		}
	})

	t.Run("undated _comment fails", func(t *testing.T) {
		mutated := mutateEndpoint(t, "claude-sonnet", func(ep map[string]any) {
			ep["_comment"] = "Anthropic Claude Sonnet pricing without a date stamp."
		})
		errs := validateBootstrapEndpointPricing(mutated, wantPrices)
		if !containsSubstring(errs, `endpoint "claude-sonnet": _comment must include a YYYY-MM-DD verification date`) {
			t.Fatalf("expected undated _comment error, got: %v", errs)
		}
	})

	t.Run("price drift from expected table fails", func(t *testing.T) {
		mutated := mutateEndpoint(t, "gemini-pro", func(ep map[string]any) {
			ep["input_price_per_1m_tokens"] = 99.0
		})
		errs := validateBootstrapEndpointPricing(mutated, wantPrices)
		if !containsSubstring(errs, `endpoint "gemini-pro": input_price_per_1m_tokens = 99.00, want 2.00`) {
			t.Fatalf("expected price drift error, got: %v", errs)
		}
	})
}

func validateBootstrapEndpointPricing(data []byte, wantPrices map[string]expectedPrice) []string {
	var errs []string

	cfg, err := config.NewLoader().LoadFromBytes(data)
	if err != nil {
		return []string{fmt.Sprintf("load config: %v", err)}
	}
	if cfg.ModelRegistry == nil {
		return []string{"model_registry is nil"}
	}
	if err := cfg.ModelRegistry.Validate(); err != nil {
		errs = append(errs, fmt.Sprintf("model_registry.Validate: %v", err))
	}

	var raw struct {
		ModelRegistry struct {
			Endpoints map[string]struct {
				Comment string `json:"_comment"`
			} `json:"endpoints"`
		} `json:"model_registry"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return []string{fmt.Sprintf("unmarshal raw model_registry: %v", err)}
	}

	if len(cfg.ModelRegistry.Endpoints) != len(wantPrices) {
		errs = append(errs, fmt.Sprintf("endpoint count = %d, want %d", len(cfg.ModelRegistry.Endpoints), len(wantPrices)))
	}

	for name, ep := range cfg.ModelRegistry.Endpoints {
		if ep == nil {
			errs = append(errs, fmt.Sprintf("endpoint %q is nil", name))
			continue
		}
		if ep.InputPricePer1MTokens <= 0 {
			errs = append(errs, fmt.Sprintf("endpoint %q: input_price_per_1m_tokens must be > 0 (got %v)", name, ep.InputPricePer1MTokens))
		}
		if ep.OutputPricePer1MTokens <= 0 {
			errs = append(errs, fmt.Sprintf("endpoint %q: output_price_per_1m_tokens must be > 0 (got %v)", name, ep.OutputPricePer1MTokens))
		}
		rawEp, ok := raw.ModelRegistry.Endpoints[name]
		if !ok || strings.TrimSpace(rawEp.Comment) == "" || !pricingDateRe.MatchString(rawEp.Comment) {
			errs = append(errs, fmt.Sprintf("endpoint %q: _comment must include a YYYY-MM-DD verification date", name))
		}
		if want, exists := wantPrices[name]; exists {
			if ep.InputPricePer1MTokens != want.in {
				errs = append(errs, fmt.Sprintf("endpoint %q: input_price_per_1m_tokens = %.2f, want %.2f", name, ep.InputPricePer1MTokens, want.in))
			}
			if ep.OutputPricePer1MTokens != want.out {
				errs = append(errs, fmt.Sprintf("endpoint %q: output_price_per_1m_tokens = %.2f, want %.2f", name, ep.OutputPricePer1MTokens, want.out))
			}
		} else {
			errs = append(errs, fmt.Sprintf("unexpected endpoint %q not in expected pricing table", name))
		}
	}

	for capName := range cfg.ModelRegistry.Capabilities {
		resolved := model.ResolveEndpointName(cfg.ModelRegistry, capName)
		if resolved == "" {
			errs = append(errs, fmt.Sprintf("capability %q failed to resolve to an endpoint", capName))
			continue
		}
		ep := cfg.ModelRegistry.GetEndpoint(resolved)
		if ep == nil || ep.InputPricePer1MTokens <= 0 || ep.OutputPricePer1MTokens <= 0 {
			errs = append(errs, fmt.Sprintf("capability %q resolved to unpriced endpoint %q", capName, resolved))
		}
	}

	return errs
}

func containsSubstring(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}
