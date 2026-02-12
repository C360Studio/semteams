package agenticgovernance

import (
	"fmt"
	"regexp"

	"github.com/c360studio/semstreams/pkg/errs"
)

// ContentFilterConfig holds content moderation filter configuration
type ContentFilterConfig struct {
	BlockThreshold float64            `json:"block_threshold" schema:"type:float,description:Block threshold (0.0-1.0),category:basic,default:0.90"`
	WarnThreshold  float64            `json:"warn_threshold" schema:"type:float,description:Warning threshold (0.0-1.0),category:basic,default:0.70"`
	Policies       []ContentPolicyDef `json:"policies,omitempty" schema:"type:array,description:Content policies,category:basic"`
	EnabledDefault []string           `json:"enabled_default,omitempty" schema:"type:array,description:Default policies to enable,category:basic"`
}

// ContentPolicyDef defines a content moderation policy
type ContentPolicyDef struct {
	Name       string       `json:"name" schema:"type:string,description:Policy identifier,category:basic"`
	Keywords   []string     `json:"keywords,omitempty" schema:"type:array,description:Keywords to match,category:basic"`
	Patterns   []string     `json:"patterns,omitempty" schema:"type:array,description:Regex patterns,category:basic"`
	Action     PolicyAction `json:"action" schema:"type:string,description:Action on violation,category:basic,default:block"`
	Severity   Severity     `json:"severity" schema:"type:string,description:Violation severity,category:basic,default:high"`
	Categories []string     `json:"categories,omitempty" schema:"type:array,description:Policy categories,category:advanced"`
}

// PolicyAction defines what happens when policy is violated
type PolicyAction string

// Policy actions define what happens when a content policy is violated.
const (
	PolicyActionBlock  PolicyAction = "block"
	PolicyActionFlag   PolicyAction = "flag"
	PolicyActionRedact PolicyAction = "redact"
)

// Validate checks content filter configuration
func (c *ContentFilterConfig) Validate() error {
	if c.BlockThreshold < 0 || c.BlockThreshold > 1 {
		return errs.WrapInvalid(fmt.Errorf("block_threshold must be between 0.0 and 1.0"), "ContentFilterConfig", "Validate", "validate block_threshold")
	}

	if c.WarnThreshold < 0 || c.WarnThreshold > 1 {
		return errs.WrapInvalid(fmt.Errorf("warn_threshold must be between 0.0 and 1.0"), "ContentFilterConfig", "Validate", "validate warn_threshold")
	}

	// Validate policy patterns compile
	for i, policy := range c.Policies {
		for j, pattern := range policy.Patterns {
			if _, err := regexp.Compile(pattern); err != nil {
				return errs.WrapInvalid(err, "ContentFilterConfig", "Validate", fmt.Sprintf("validate policies[%d].patterns[%d] regex", i, j))
			}
		}
	}

	return nil
}

// DefaultContentConfig returns default content filter configuration
func DefaultContentConfig() *ContentFilterConfig {
	return &ContentFilterConfig{
		BlockThreshold: 0.90,
		WarnThreshold:  0.70,
		EnabledDefault: []string{"harmful", "illegal"},
	}
}
