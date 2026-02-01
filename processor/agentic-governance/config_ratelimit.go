package agenticgovernance

import "fmt"

// RateLimitFilterConfig holds rate limiting filter configuration
type RateLimitFilterConfig struct {
	PerUser    RateLimitDef     `json:"per_user" schema:"type:object,description:Per-user rate limits,category:basic"`
	PerSession RateLimitDef     `json:"per_session,omitempty" schema:"type:object,description:Per-session rate limits,category:basic"`
	Global     RateLimitDef     `json:"global,omitempty" schema:"type:object,description:Global rate limits,category:basic"`
	Algorithm  RateLimitAlgo    `json:"algorithm" schema:"type:string,description:Rate limiting algorithm,category:advanced,default:token_bucket"`
	Storage    RateLimitStorage `json:"storage,omitempty" schema:"type:object,description:Storage configuration,category:advanced"`
}

// RateLimitDef defines rate limits for a scope
type RateLimitDef struct {
	RequestsPerMinute int `json:"requests_per_minute" schema:"type:int,description:Maximum requests per minute,category:basic,default:60"`
	TokensPerHour     int `json:"tokens_per_hour,omitempty" schema:"type:int,description:Maximum tokens per hour,category:basic,default:100000"`
}

// RateLimitAlgo specifies the rate limiting algorithm
type RateLimitAlgo string

// Rate limiting algorithms define how rate limits are enforced.
const (
	AlgoTokenBucket   RateLimitAlgo = "token_bucket"
	AlgoSlidingWindow RateLimitAlgo = "sliding_window"
)

// RateLimitStorage configures rate limit state storage
type RateLimitStorage struct {
	Type   string `json:"type" schema:"type:string,description:Storage type (memory kv),category:advanced,default:memory"`
	Bucket string `json:"bucket,omitempty" schema:"type:string,description:KV bucket name,category:advanced"`
}

// Validate checks rate limit filter configuration
func (c *RateLimitFilterConfig) Validate() error {
	if c.PerUser.RequestsPerMinute < 0 {
		return fmt.Errorf("per_user.requests_per_minute cannot be negative")
	}

	if c.PerUser.TokensPerHour < 0 {
		return fmt.Errorf("per_user.tokens_per_hour cannot be negative")
	}

	return nil
}

// DefaultRateLimitConfig returns default rate limit filter configuration
func DefaultRateLimitConfig() *RateLimitFilterConfig {
	return &RateLimitFilterConfig{
		PerUser: RateLimitDef{
			RequestsPerMinute: 60,
			TokensPerHour:     100000,
		},
		PerSession: RateLimitDef{
			RequestsPerMinute: 30,
			TokensPerHour:     50000,
		},
		Global: RateLimitDef{
			RequestsPerMinute: 1000,
			TokensPerHour:     1000000,
		},
		Algorithm: AlgoTokenBucket,
		Storage: RateLimitStorage{
			Type: "memory",
		},
	}
}
