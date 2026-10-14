package githubread

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CursorEntry tracks the high-water mark and per-origin version digests for a
// single (repository, kind) stream so repeated runs do not re-emit unchanged
// GitHub activity (#273).
type CursorEntry struct {
	Watermark time.Time         `json:"watermark"`
	Seen      map[string]string `json:"seen,omitempty"`
}

func (c CursorEntry) clone() CursorEntry {
	seen := make(map[string]string, len(c.Seen))
	for k, v := range c.Seen {
		seen[k] = v
	}
	return CursorEntry{
		Watermark: c.Watermark,
		Seen:      seen,
	}
}

// CursorStore persists per-(repository, kind) cursor state across github_read calls.
type CursorStore interface {
	Get(repoKey, kind string) (CursorEntry, bool)
	Put(repoKey, kind string, entry CursorEntry) error
}

// VersionDigest computes a deterministic version token for a retrieved GitHub
// object combining its activity timestamp and a SHA-256 prefix of its raw body.
func VersionDigest(activityAt time.Time, rawBody string) string {
	sum := sha256.Sum256([]byte(rawBody))
	return fmt.Sprintf("%s#%s", activityAt.UTC().Format(time.RFC3339Nano), hex.EncodeToString(sum[:8]))
}

// MemoryCursorStore is an in-memory thread-safe CursorStore.
type MemoryCursorStore struct {
	mu      sync.RWMutex
	entries map[string]map[string]CursorEntry
}

// NewMemoryCursorStore creates an empty MemoryCursorStore.
func NewMemoryCursorStore() *MemoryCursorStore {
	return &MemoryCursorStore{
		entries: make(map[string]map[string]CursorEntry),
	}
}

// Get returns the stored CursorEntry for (repoKey, kind) if present.
func (s *MemoryCursorStore) Get(repoKey, kind string) (CursorEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byKind, ok := s.entries[repoKey]
	if !ok {
		return CursorEntry{}, false
	}
	entry, ok := byKind[kind]
	if !ok {
		return CursorEntry{}, false
	}
	return entry.clone(), true
}

// Put stores the CursorEntry for (repoKey, kind).
func (s *MemoryCursorStore) Put(repoKey, kind string, entry CursorEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries[repoKey] == nil {
		s.entries[repoKey] = make(map[string]CursorEntry)
	}
	s.entries[repoKey][kind] = entry.clone()
	return nil
}

// FileCursorStore persists cursor state to a JSON file on disk using atomic
// temp-file + rename writes.
type FileCursorStore struct {
	mu   sync.Mutex
	path string
	mem  *MemoryCursorStore
}

// NewFileCursorStore loads or initializes a JSON cursor store at path.
func NewFileCursorStore(path string) (*FileCursorStore, error) {
	mem := NewMemoryCursorStore()
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		var loaded map[string]map[string]CursorEntry
		if err := json.Unmarshal(data, &loaded); err != nil {
			return nil, fmt.Errorf("decode cursor store %s: %w", path, err)
		}
		mem.entries = loaded
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read cursor store %s: %w", path, err)
	}
	return &FileCursorStore{
		path: path,
		mem:  mem,
	}, nil
}

// Get returns the stored CursorEntry for (repoKey, kind).
func (s *FileCursorStore) Get(repoKey, kind string) (CursorEntry, bool) {
	return s.mem.Get(repoKey, kind)
}

// Put updates the in-memory state and atomically flushes the JSON file to disk.
func (s *FileCursorStore) Put(repoKey, kind string, entry CursorEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mem.Put(repoKey, kind, entry); err != nil {
		return err
	}
	s.mem.mu.RLock()
	data, err := json.MarshalIndent(s.mem.entries, "", "  ")
	s.mem.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("encode cursor store: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir cursor store dir %s: %w", dir, err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write cursor store temp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename cursor store %s: %w", s.path, err)
	}
	return nil
}
