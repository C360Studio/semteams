package curator

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// Ensure Artifact implements message.Payload at compile time.
var _ message.Payload = (*Artifact)(nil)

// validArtifact returns a fully-populated Artifact for happy-path tests.
func validArtifact() *Artifact {
	return &Artifact{
		LoopID: "loop-curator-abc",
		AddedSources: []AddedSource{
			{
				URL:       "https://github.com/sensorhub-tools/osh-core",
				Namespace: "osh-core",
				Covers:    "OSH IDriver interface, IPersistentDriver, BaseSensorModule",
			},
		},
		VerifiedEntityIDs: []string{
			"osh-core.org.sensorhub.api.module.IModule",
			"osh-core.org.sensorhub.api.module.IModuleProvider",
		},
		ProducedAt: time.Now().UTC(),
	}
}

// validArtifactWithSourceDirs returns an Artifact that includes source_dirs.
func validArtifactWithSourceDirs() *Artifact {
	a := validArtifact()
	a.SourceDirs = []SourceDir{
		{
			Namespace: "osh-core",
			MountPath: "/sources/osh-core",
			Covers:    "OSH module/driver interfaces, gradle build wiring",
		},
	}
	return a
}

func TestValidate_HappyPath(t *testing.T) {
	if err := validArtifact().Validate(); err != nil {
		t.Errorf("valid artifact failed validation: %v", err)
	}
}

func TestValidate_HappyPathWithSourceDirs(t *testing.T) {
	if err := validArtifactWithSourceDirs().Validate(); err != nil {
		t.Errorf("valid artifact with source_dirs failed validation: %v", err)
	}
}

func TestValidate_Cases(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Artifact)
		wantErr string
	}{
		{
			name:    "missing loop_id",
			mutate:  func(a *Artifact) { a.LoopID = "" },
			wantErr: "loop_id required",
		},
		{
			name:    "empty added_sources",
			mutate:  func(a *Artifact) { a.AddedSources = nil },
			wantErr: "added_sources required",
		},
		{
			name: "added_source missing url",
			mutate: func(a *Artifact) {
				a.AddedSources[0].URL = ""
			},
			wantErr: "added_sources[0].url required",
		},
		{
			name: "added_source missing namespace",
			mutate: func(a *Artifact) {
				a.AddedSources[0].Namespace = ""
			},
			wantErr: "added_sources[0].namespace required",
		},
		{
			name: "added_source missing covers",
			mutate: func(a *Artifact) {
				a.AddedSources[0].Covers = ""
			},
			wantErr: "added_sources[0].covers required",
		},
		{
			name:    "empty verified_entity_ids",
			mutate:  func(a *Artifact) { a.VerifiedEntityIDs = nil },
			wantErr: "verified_entity_ids required",
		},
		{
			name: "source_dirs entry missing namespace",
			mutate: func(a *Artifact) {
				a.SourceDirs = []SourceDir{{Namespace: "", MountPath: "/sources/osh-core", Covers: "stuff"}}
			},
			wantErr: "source_dirs[0].namespace required",
		},
		{
			name: "source_dirs entry missing mount_path",
			mutate: func(a *Artifact) {
				a.SourceDirs = []SourceDir{{Namespace: "osh-core", MountPath: "", Covers: "stuff"}}
			},
			wantErr: "source_dirs[0].mount_path required",
		},
		{
			name: "source_dirs entry missing covers",
			mutate: func(a *Artifact) {
				a.SourceDirs = []SourceDir{{Namespace: "osh-core", MountPath: "/sources/osh-core", Covers: ""}}
			},
			wantErr: "source_dirs[0].covers required",
		},
		{
			name:    "zero produced_at",
			mutate:  func(a *Artifact) { a.ProducedAt = time.Time{} },
			wantErr: "produced_at required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := validArtifact()
			tc.mutate(a)
			err := a.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if tc.wantErr != "" && !containsStr(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestSchema(t *testing.T) {
	a := &Artifact{}
	got := a.Schema()
	want := message.Type{Domain: "curator", Category: "artifact", Version: "v1"}
	if got != want {
		t.Errorf("Schema() = %+v, want %+v", got, want)
	}
}

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	orig := validArtifactWithSourceDirs()
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Artifact
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.LoopID != orig.LoopID {
		t.Errorf("LoopID = %q, want %q", got.LoopID, orig.LoopID)
	}
	if len(got.AddedSources) != len(orig.AddedSources) {
		t.Errorf("AddedSources len = %d, want %d", len(got.AddedSources), len(orig.AddedSources))
	} else {
		if got.AddedSources[0].URL != orig.AddedSources[0].URL {
			t.Errorf("AddedSources[0].URL = %q, want %q", got.AddedSources[0].URL, orig.AddedSources[0].URL)
		}
		if got.AddedSources[0].Namespace != orig.AddedSources[0].Namespace {
			t.Errorf("AddedSources[0].Namespace = %q, want %q", got.AddedSources[0].Namespace, orig.AddedSources[0].Namespace)
		}
		if got.AddedSources[0].Covers != orig.AddedSources[0].Covers {
			t.Errorf("AddedSources[0].Covers = %q, want %q", got.AddedSources[0].Covers, orig.AddedSources[0].Covers)
		}
	}
	if len(got.VerifiedEntityIDs) != len(orig.VerifiedEntityIDs) {
		t.Errorf("VerifiedEntityIDs len = %d, want %d", len(got.VerifiedEntityIDs), len(orig.VerifiedEntityIDs))
	}
	if len(got.SourceDirs) != len(orig.SourceDirs) {
		t.Errorf("SourceDirs len = %d, want %d", len(got.SourceDirs), len(orig.SourceDirs))
	} else {
		if got.SourceDirs[0].Namespace != orig.SourceDirs[0].Namespace {
			t.Errorf("SourceDirs[0].Namespace = %q, want %q", got.SourceDirs[0].Namespace, orig.SourceDirs[0].Namespace)
		}
		if got.SourceDirs[0].MountPath != orig.SourceDirs[0].MountPath {
			t.Errorf("SourceDirs[0].MountPath = %q, want %q", got.SourceDirs[0].MountPath, orig.SourceDirs[0].MountPath)
		}
	}
	if !got.ProducedAt.Equal(orig.ProducedAt) {
		t.Errorf("ProducedAt = %v, want %v", got.ProducedAt, orig.ProducedAt)
	}
}

func TestMarshalUnmarshal_OmitemptySourceDirs(t *testing.T) {
	// Without source_dirs the JSON must not contain the key at all.
	a := validArtifact() // no SourceDirs
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if _, ok := raw["source_dirs"]; ok {
		t.Error("source_dirs must be omitted when empty (omitempty)")
	}
}

func TestRegisterPayloads(t *testing.T) {
	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}
	// Calling again should error — duplicate registration is a
	// boot-time misconfiguration (mirrors research.TestRegisterPayloads).
	if err := RegisterPayloads(reg); err == nil {
		t.Errorf("RegisterPayloads: expected duplicate-registration error on second call")
	}

	// Verify the factory returns an *Artifact.
	got := reg.Create("curator", "artifact", "v1")
	if _, ok := got.(*Artifact); !ok {
		t.Errorf("factory returned %T, want *Artifact", got)
	}
}

func TestRegisterPayloads_FactoryRoundtrip(t *testing.T) {
	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}

	orig := validArtifactWithSourceDirs()
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Create a fresh instance via the registry factory and unmarshal into it.
	raw := reg.Create("curator", "artifact", "v1")
	got, ok := raw.(*Artifact)
	if !ok {
		t.Fatalf("factory type = %T, want *Artifact", raw)
	}
	if err := json.Unmarshal(data, got); err != nil {
		t.Fatalf("Unmarshal into factory instance: %v", err)
	}
	if got.LoopID != orig.LoopID {
		t.Errorf("roundtrip LoopID = %q, want %q", got.LoopID, orig.LoopID)
	}
	if len(got.AddedSources) != len(orig.AddedSources) {
		t.Errorf("roundtrip AddedSources len = %d, want %d", len(got.AddedSources), len(orig.AddedSources))
	}
	if len(got.SourceDirs) != len(orig.SourceDirs) {
		t.Errorf("roundtrip SourceDirs len = %d, want %d", len(got.SourceDirs), len(orig.SourceDirs))
	}
}

// containsStr is a helper to avoid importing strings in test file.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
