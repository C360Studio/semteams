package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// bucketName is the NATS KV bucket where harness catalog entries
// persist. History:5 mirrors flowtemplate / flowstore so operators
// can audit recent catalog edits.
const bucketName = "HARNESSES"

// Manager provides read access to the harness catalog backed by a
// NATS KV bucket. Writes happen via LoadFromFile (operator-curated
// path) and Put (forward-compat for R3.7.4 candidate promotion);
// CRUD exposure stays minimal in v1 — no public Update / Delete
// because nothing in this slice authors catalog entries from agent
// loops.
type Manager struct {
	kv *natsclient.KVStore
}

// NewManager opens (or creates) the HARNESSES KV bucket.
func NewManager(natsClient *natsclient.Client) (*Manager, error) {
	if natsClient == nil {
		return nil, fmt.Errorf("nats client cannot be nil")
	}
	bucket, err := natsClient.CreateKeyValueBucket(context.Background(), jetstream.KeyValueConfig{
		Bucket:      bucketName,
		Description: "Harness catalog: operator-curated test harnesses for dev-via-spec verification (ADR-033)",
		History:     5,
	})
	if err != nil {
		return nil, fmt.Errorf("create harness KV bucket: %w", err)
	}
	return &Manager{kv: natsClient.NewKVStore(bucket)}, nil
}

// Get retrieves a harness by name. Returns an error wrapping
// jetstream.ErrKeyNotFound if the harness is not registered.
func (m *Manager) Get(ctx context.Context, name string) (*Harness, error) {
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}
	entry, err := m.kv.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get harness %q: %w", name, err)
	}
	var h Harness
	if err := json.Unmarshal(entry.Value, &h); err != nil {
		return nil, fmt.Errorf("unmarshal harness %q: %w", name, err)
	}
	return &h, nil
}

// List returns every harness sorted by name. The catalog is small
// (typically <20 entries per deployment) — no pagination.
func (m *Manager) List(ctx context.Context) ([]*Harness, error) {
	keys, err := m.kv.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list harness keys: %w", err)
	}
	sort.Strings(keys)
	out := make([]*Harness, 0, len(keys))
	for _, k := range keys {
		h, err := m.Get(ctx, k)
		if err != nil {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}

// Put writes a harness entry. Forward-compat for R3.7.4 (coordinator
// promotes harness-via-spec candidates into the catalog). In R3.7.1
// this is exercised only from LoadFromFile.
func (m *Manager) Put(ctx context.Context, h *Harness) error {
	if h == nil {
		return fmt.Errorf("harness cannot be nil")
	}
	if err := h.Validate(); err != nil {
		return fmt.Errorf("validate harness %q: %w", h.Name, err)
	}
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshal harness %q: %w", h.Name, err)
	}
	if _, err := m.kv.Put(ctx, h.Name, data); err != nil {
		return fmt.Errorf("put harness %q: %w", h.Name, err)
	}
	return nil
}

// LoadFromFile reads `configs/harnesses.json` (or any path), parses
// it, and writes each entry into the KV bucket. Existing KV entries
// not present in the file are NOT removed in this slice — operators
// can use semstreams admin tooling, or `Delete` lands when R3.7.4
// promotion churns the catalog. Returns the number of entries
// loaded.
//
// A missing file is NOT an error: the catalog starts empty and
// operators add the file when they curate their first harness. This
// matches the boot-time wiring: deployments that don't use harnesses
// shouldn't fail to boot.
func (m *Manager) LoadFromFile(ctx context.Context, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read catalog file %q: %w", path, err)
	}
	entries, err := ParseFile(data)
	if err != nil {
		return 0, fmt.Errorf("parse catalog file %q: %w", path, err)
	}
	for i := range entries {
		if err := m.Put(ctx, &entries[i]); err != nil {
			return 0, fmt.Errorf("load entry %d (%q): %w", i, entries[i].Name, err)
		}
	}
	return len(entries), nil
}
