package trustgraph

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Endpoint != "http://localhost:8088" {
		t.Errorf("Expected default endpoint http://localhost:8088, got %s", cfg.Endpoint)
	}
	if cfg.PollInterval != "60s" {
		t.Errorf("Expected default poll_interval 60s, got %s", cfg.PollInterval)
	}
	if cfg.Limit != 1000 {
		t.Errorf("Expected default limit 1000, got %d", cfg.Limit)
	}
	if cfg.Source != "trustgraph" {
		t.Errorf("Expected default source trustgraph, got %s", cfg.Source)
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
			name: "invalid poll interval",
			config: Config{
				PollInterval: "invalid",
				Ports:        DefaultConfig().Ports,
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
			name: "negative limit",
			config: Config{
				Limit: -1,
				Ports: DefaultConfig().Ports,
			},
			wantErr: true,
		},
		{
			name: "missing output port",
			config: Config{
				Ports: nil,
			},
			wantErr: false, // nil ports is OK, defaults will be used
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

func TestConfig_GetPollInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		wantSecs float64
	}{
		{"default empty", "", 60},
		{"30 seconds", "30s", 30},
		{"5 minutes", "5m", 300},
		{"invalid falls back", "invalid", 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{PollInterval: tt.interval}
			got := cfg.GetPollInterval().Seconds()
			if got != tt.wantSecs {
				t.Errorf("GetPollInterval() = %v seconds, want %v", got, tt.wantSecs)
			}
		})
	}
}

func TestConfig_GetOutputSubject(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{
			name:   "nil ports",
			config: Config{},
			want:   "entity.>",
		},
		{
			name:   "configured subject",
			config: DefaultConfig(),
			want:   "entity.>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetOutputSubject()
			if got != tt.want {
				t.Errorf("GetOutputSubject() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComponent_Meta(t *testing.T) {
	c := &Component{
		name: "test-input",
		config: Config{
			Endpoint: "http://localhost:8088",
		},
	}

	meta := c.Meta()

	if meta.Name != "test-input" {
		t.Errorf("Meta().Name = %v, want test-input", meta.Name)
	}
	if meta.Type != "input" {
		t.Errorf("Meta().Type = %v, want input", meta.Type)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Meta().Version = %v, want 1.0.0", meta.Version)
	}
}

func TestComponent_InputPorts(t *testing.T) {
	c := &Component{
		config: Config{
			Endpoint: "http://trustgraph:8088",
		},
	}

	ports := c.InputPorts()

	if len(ports) != 1 {
		t.Fatalf("Expected 1 input port, got %d", len(ports))
	}

	if ports[0].Name != "trustgraph_api" {
		t.Errorf("Input port name = %v, want trustgraph_api", ports[0].Name)
	}
}

func TestComponent_OutputPorts(t *testing.T) {
	c := &Component{
		config: DefaultConfig(),
	}

	ports := c.OutputPorts()

	if len(ports) != 1 {
		t.Fatalf("Expected 1 output port, got %d", len(ports))
	}

	if ports[0].Name != "entity" {
		t.Errorf("Output port name = %v, want entity", ports[0].Name)
	}
}
