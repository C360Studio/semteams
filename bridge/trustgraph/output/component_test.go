package output

import (
	"testing"

	"github.com/c360studio/semstreams/message"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Endpoint != "http://localhost:8088" {
		t.Errorf("Expected default endpoint http://localhost:8088, got %s", cfg.Endpoint)
	}
	if cfg.KGCoreID != "semstreams-operational" {
		t.Errorf("Expected default kg_core_id semstreams-operational, got %s", cfg.KGCoreID)
	}
	if cfg.User != "semstreams" {
		t.Errorf("Expected default user semstreams, got %s", cfg.User)
	}
	if cfg.Collection != "operational" {
		t.Errorf("Expected default collection operational, got %s", cfg.Collection)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("Expected default batch_size 100, got %d", cfg.BatchSize)
	}
	if len(cfg.ExcludeSources) != 1 || cfg.ExcludeSources[0] != "trustgraph" {
		t.Errorf("Expected default exclude_sources [trustgraph], got %v", cfg.ExcludeSources)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "default config valid",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid flush interval",
			config: Config{
				FlushInterval: "invalid",
				Ports:         DefaultConfig().Ports,
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			config: Config{
				Timeout: "invalid",
				Ports:   DefaultConfig().Ports,
			},
			wantErr: true,
		},
		{
			name: "negative batch_size",
			config: Config{
				BatchSize: -1,
				Ports:     DefaultConfig().Ports,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_GetFlushInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		wantSecs float64
	}{
		{"default empty", "", 5},
		{"10 seconds", "10s", 10},
		{"1 minute", "1m", 60},
		{"invalid falls back", "invalid", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{FlushInterval: tt.interval}
			got := cfg.GetFlushInterval().Seconds()
			if got != tt.wantSecs {
				t.Errorf("GetFlushInterval() = %v seconds, want %v", got, tt.wantSecs)
			}
		})
	}
}

func TestComponent_Meta(t *testing.T) {
	c := &Component{
		name: "test-output",
		config: Config{
			Endpoint: "http://localhost:8088",
		},
	}

	meta := c.Meta()

	if meta.Name != "test-output" {
		t.Errorf("Meta().Name = %v, want test-output", meta.Name)
	}
	if meta.Type != "output" {
		t.Errorf("Meta().Type = %v, want output", meta.Type)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Meta().Version = %v, want 1.0.0", meta.Version)
	}
}

func TestComponent_InputPorts(t *testing.T) {
	c := &Component{
		config: DefaultConfig(),
	}

	ports := c.InputPorts()

	if len(ports) != 1 {
		t.Fatalf("Expected 1 input port, got %d", len(ports))
	}

	if ports[0].Name != "entity" {
		t.Errorf("Input port name = %v, want entity", ports[0].Name)
	}
}

func TestComponent_OutputPorts(t *testing.T) {
	c := &Component{
		config: Config{
			Endpoint: "http://trustgraph:8088",
		},
	}

	ports := c.OutputPorts()

	if len(ports) != 1 {
		t.Fatalf("Expected 1 output port, got %d", len(ports))
	}

	if ports[0].Name != "trustgraph_api" {
		t.Errorf("Output port name = %v, want trustgraph_api", ports[0].Name)
	}
}

func TestComponent_matchesPrefix(t *testing.T) {
	tests := []struct {
		name     string
		prefixes []string
		entityID string
		want     bool
	}{
		{
			name:     "no filter accepts all",
			prefixes: nil,
			entityID: "any.entity.id.here.type.instance",
			want:     true,
		},
		{
			name:     "empty filter accepts all",
			prefixes: []string{},
			entityID: "any.entity.id.here.type.instance",
			want:     true,
		},
		{
			name:     "matching prefix",
			prefixes: []string{"acme.ops."},
			entityID: "acme.ops.robotics.gcs.drone.001",
			want:     true,
		},
		{
			name:     "non-matching prefix",
			prefixes: []string{"acme.ops."},
			entityID: "other.platform.domain.system.type.instance",
			want:     false,
		},
		{
			name:     "one of multiple prefixes",
			prefixes: []string{"acme.ops.", "client.intel."},
			entityID: "client.intel.knowledge.entity.concept.test",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Component{
				config: Config{
					EntityPrefixes: tt.prefixes,
				},
			}
			if got := c.matchesPrefix(tt.entityID); got != tt.want {
				t.Errorf("matchesPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComponent_shouldExclude(t *testing.T) {
	tests := []struct {
		name           string
		excludeSources []string
		triples        []message.Triple
		want           bool
	}{
		{
			name:           "no exclusion filter",
			excludeSources: nil,
			triples: []message.Triple{
				{Source: "trustgraph"},
			},
			want: false,
		},
		{
			name:           "all triples from excluded source",
			excludeSources: []string{"trustgraph"},
			triples: []message.Triple{
				{Source: "trustgraph"},
				{Source: "trustgraph"},
			},
			want: true,
		},
		{
			name:           "mixed sources - not excluded",
			excludeSources: []string{"trustgraph"},
			triples: []message.Triple{
				{Source: "trustgraph"},
				{Source: "sensor"},
			},
			want: false,
		},
		{
			name:           "no triples from excluded source",
			excludeSources: []string{"trustgraph"},
			triples: []message.Triple{
				{Source: "sensor"},
				{Source: "operator"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Component{
				config: Config{
					ExcludeSources: tt.excludeSources,
				},
			}
			if got := c.shouldExclude(tt.triples); got != tt.want {
				t.Errorf("shouldExclude() = %v, want %v", got, tt.want)
			}
		})
	}
}
