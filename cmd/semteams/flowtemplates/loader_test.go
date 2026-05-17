package flowtemplates

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/flowtemplate"
)

// fakeManager is an in-memory Manager for unit tests. Records every Create
// and Update call so tests can assert idempotent re-seed behavior.
type fakeManager struct {
	mu       sync.Mutex
	store    map[string]*flowtemplate.Template
	creates  []string
	updates  []string
	getErr   error
	createFn func(t *flowtemplate.Template) error
	updateFn func(t *flowtemplate.Template) error
}

func newFakeManager() *fakeManager {
	return &fakeManager{store: make(map[string]*flowtemplate.Template)}
}

func (f *fakeManager) Get(_ context.Context, id string) (*flowtemplate.Template, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	t, ok := f.store[id]
	if !ok {
		// Match natsclient.IsKVNotFoundError's substring detection so the
		// loader's upsert routes this through Create rather than treating
		// it as a transient failure.
		return nil, errors.New("key not found")
	}
	return t, nil
}

func (f *fakeManager) Create(_ context.Context, t *flowtemplate.Template) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createFn != nil {
		if err := f.createFn(t); err != nil {
			return err
		}
	}
	f.creates = append(f.creates, t.ID)
	f.store[t.ID] = t
	return nil
}

func (f *fakeManager) Update(_ context.Context, t *flowtemplate.Template) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateFn != nil {
		if err := f.updateFn(t); err != nil {
			return err
		}
	}
	f.updates = append(f.updates, t.ID)
	f.store[t.ID] = t
	return nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func writeTemplate(t *testing.T, dir, fileName string, tmpl *flowtemplate.Template) {
	t.Helper()
	data, err := json.Marshal(tmpl)
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), data, 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
}

func TestLoadFromDirectory_MissingDir(t *testing.T) {
	mgr := newFakeManager()
	err := LoadFromDirectory(context.Background(), "/nonexistent/path/that/does/not/exist", mgr, quietLogger())
	if err != nil {
		t.Fatalf("missing dir should be non-fatal, got: %v", err)
	}
	if len(mgr.creates)+len(mgr.updates) != 0 {
		t.Fatalf("missing dir should not produce any upserts; got creates=%v updates=%v", mgr.creates, mgr.updates)
	}
}

func TestLoadFromDirectory_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr := newFakeManager()

	err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger())
	if err != nil {
		t.Fatalf("empty dir should be non-fatal, got: %v", err)
	}
	if len(mgr.creates)+len(mgr.updates) != 0 {
		t.Fatalf("empty dir should not produce any upserts; got creates=%v updates=%v", mgr.creates, mgr.updates)
	}
}

func TestLoadFromDirectory_NilManager(t *testing.T) {
	// Should be a no-op, not a panic.
	err := LoadFromDirectory(context.Background(), t.TempDir(), nil, quietLogger())
	if err != nil {
		t.Fatalf("nil manager should be non-fatal, got: %v", err)
	}
}

func TestLoadFromDirectory_LoadsValidTemplate(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "alpha.json", &flowtemplate.Template{
		ID:   "alpha",
		Name: "Alpha template",
		Body: `{"id":"flow-{{.Topic}}","name":"alpha","nodes":[],"connections":[]}`,
		Parameters: []flowtemplate.Parameter{
			{Name: "Topic", Default: "default-topic"},
		},
	})

	mgr := newFakeManager()
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	if len(mgr.creates) != 1 || mgr.creates[0] != "alpha" {
		t.Fatalf("expected one create for alpha, got creates=%v updates=%v", mgr.creates, mgr.updates)
	}
}

func TestLoadFromDirectory_IdempotentReseed(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "alpha.json", &flowtemplate.Template{
		ID:   "alpha",
		Name: "Alpha template",
		Body: `{"id":"flow","name":"alpha","nodes":[],"connections":[]}`,
	})

	mgr := newFakeManager()
	// First load — Create.
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("first LoadFromDirectory: %v", err)
	}
	// Second load — must Update, not Create again.
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("second LoadFromDirectory: %v", err)
	}

	if len(mgr.creates) != 1 {
		t.Fatalf("expected exactly one create across two loads, got %v", mgr.creates)
	}
	if len(mgr.updates) != 1 || mgr.updates[0] != "alpha" {
		t.Fatalf("expected one update on second load, got %v", mgr.updates)
	}
}

func TestLoadFromDirectory_SkipsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write broken: %v", err)
	}
	writeTemplate(t, dir, "good.json", &flowtemplate.Template{
		ID:   "good",
		Name: "Good template",
		Body: `{"id":"flow","name":"good","nodes":[],"connections":[]}`,
	})

	mgr := newFakeManager()
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("malformed JSON should be non-fatal at the loader level, got: %v", err)
	}

	if len(mgr.creates) != 1 || mgr.creates[0] != "good" {
		t.Fatalf("expected only good template to load, got creates=%v", mgr.creates)
	}
}

func TestLoadFromDirectory_SkipsValidationFailure(t *testing.T) {
	dir := t.TempDir()
	// Template missing required ID — Validate() should reject.
	writeTemplate(t, dir, "no-id.json", &flowtemplate.Template{
		Name: "Missing ID",
		Body: `{"id":"flow","name":"x","nodes":[],"connections":[]}`,
	})
	writeTemplate(t, dir, "good.json", &flowtemplate.Template{
		ID:   "good",
		Name: "Good",
		Body: `{"id":"flow","name":"good","nodes":[],"connections":[]}`,
	})

	mgr := newFakeManager()
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("validation failure should be non-fatal at the loader level, got: %v", err)
	}

	if len(mgr.creates) != 1 || mgr.creates[0] != "good" {
		t.Fatalf("expected only good template to load, got creates=%v", mgr.creates)
	}
}

func TestLoadFromDirectory_IgnoresNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "alpha.json", &flowtemplate.Template{
		ID:   "alpha",
		Name: "Alpha",
		Body: `{"id":"flow","name":"alpha","nodes":[],"connections":[]}`,
	})
	// README, dotfiles, and other extensions must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs"), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte(""), 0o600); err != nil {
		t.Fatalf("write gitkeep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	mgr := newFakeManager()
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	if len(mgr.creates) != 1 || mgr.creates[0] != "alpha" {
		t.Fatalf("expected only alpha to load, got creates=%v", mgr.creates)
	}
}

func TestLoadFromDirectory_SkipsNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	writeTemplate(t, nested, "inner.json", &flowtemplate.Template{
		ID:   "inner",
		Name: "Inner",
		Body: `{"id":"flow","name":"inner","nodes":[],"connections":[]}`,
	})
	writeTemplate(t, dir, "outer.json", &flowtemplate.Template{
		ID:   "outer",
		Name: "Outer",
		Body: `{"id":"flow","name":"outer","nodes":[],"connections":[]}`,
	})

	mgr := newFakeManager()
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	if len(mgr.creates) != 1 || mgr.creates[0] != "outer" {
		t.Fatalf("expected only outer to load (nested dirs skipped), got creates=%v", mgr.creates)
	}
}

func TestLoadFromDirectory_ReturnsFirstUpsertError(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "alpha.json", &flowtemplate.Template{
		ID:   "alpha",
		Name: "Alpha",
		Body: `{"id":"flow","name":"alpha","nodes":[],"connections":[]}`,
	})
	writeTemplate(t, dir, "beta.json", &flowtemplate.Template{
		ID:   "beta",
		Name: "Beta",
		Body: `{"id":"flow","name":"beta","nodes":[],"connections":[]}`,
	})

	mgr := newFakeManager()
	wantErr := errors.New("kv unavailable")
	mgr.createFn = func(_ *flowtemplate.Template) error { return wantErr }

	err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected first upsert error %v, got %v", wantErr, err)
	}
}

func TestLoadFromDirectory_ReportsUpdateFailure(t *testing.T) {
	// Pre-seed the fake so the second load goes through Update, then
	// assert an Update error propagates as the loader's first-upsert-err.
	dir := t.TempDir()
	writeTemplate(t, dir, "alpha.json", &flowtemplate.Template{
		ID:   "alpha",
		Name: "Alpha",
		Body: `{"id":"flow","name":"alpha","nodes":[],"connections":[]}`,
	})

	mgr := newFakeManager()
	// First load — Create populates the fake.
	if err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger()); err != nil {
		t.Fatalf("first LoadFromDirectory: %v", err)
	}

	// Arm Update to fail on the second load.
	wantErr := errors.New("update failed")
	mgr.updateFn = func(_ *flowtemplate.Template) error { return wantErr }

	err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected Update error to surface via firstUpsertErr, got: %v", err)
	}
}

func TestLoadFromDirectory_SurfacesTransientGetError(t *testing.T) {
	// Validates the M1 fix: a non-NotFound Get error must NOT be
	// silently coerced to a Create attempt. Use a getErr that lacks
	// the "key not found" sentinel — natsclient.IsKVNotFoundError
	// returns false → upsert surfaces the error directly.
	dir := t.TempDir()
	writeTemplate(t, dir, "alpha.json", &flowtemplate.Template{
		ID:   "alpha",
		Name: "Alpha",
		Body: `{"id":"flow","name":"alpha","nodes":[],"connections":[]}`,
	})

	mgr := newFakeManager()
	mgr.getErr = errors.New("kv transient: server timeout")

	err := LoadFromDirectory(context.Background(), dir, mgr, quietLogger())
	if err == nil {
		t.Fatalf("expected transient Get error to propagate, got nil")
	}
	// Create must not have been attempted — the transient Get error is
	// terminal for this upsert, no silent fallback.
	if len(mgr.creates) != 0 {
		t.Fatalf("transient Get error must not route to Create; got creates=%v", mgr.creates)
	}
}

func TestLoadFromDirectory_HonoursCtxCancellation(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "alpha.json", &flowtemplate.Template{
		ID:   "alpha",
		Name: "Alpha",
		Body: `{"id":"flow","name":"alpha","nodes":[],"connections":[]}`,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mgr := newFakeManager()
	err := LoadFromDirectory(ctx, dir, mgr, quietLogger())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled to surface, got: %v", err)
	}
	if len(mgr.creates)+len(mgr.updates) != 0 {
		t.Fatalf("cancelled context should not produce upserts, got creates=%v updates=%v", mgr.creates, mgr.updates)
	}
}
