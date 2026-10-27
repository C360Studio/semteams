package githubread

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func normalizeRepoKey(repoKey string) string {
	return strings.ToLower(strings.TrimSpace(repoKey))
}

// CursorEntry tracks the high-water mark, per-origin version digests, baseline
// onboarding state, and check vs. persist timestamps for a single
// (repository, kind) stream so repeated runs do not re-emit unchanged GitHub
// activity and failed runs do not lose deltas (#273, #268).
type CursorEntry struct {
	Watermark           time.Time         `json:"watermark"`
	Seen                map[string]string `json:"seen,omitempty"`
	BaselineEstablished bool              `json:"baseline_established,omitempty"`
	LastCheckedAt       time.Time         `json:"last_checked_at,omitempty"`
	LastPersistedAt     time.Time         `json:"last_persisted_at,omitempty"`
}

func (c CursorEntry) clone() CursorEntry {
	seen := make(map[string]string, len(c.Seen))
	for k, v := range c.Seen {
		seen[k] = v
	}
	return CursorEntry{
		Watermark:           c.Watermark,
		Seen:                seen,
		BaselineEstablished: c.BaselineEstablished,
		LastCheckedAt:       c.LastCheckedAt,
		LastPersistedAt:     c.LastPersistedAt,
	}
}

// isBaselined reports whether a CursorEntry has completed initial onboarding.
func (c CursorEntry) isBaselined() bool {
	return c.BaselineEstablished || len(c.Seen) > 0 || !c.Watermark.IsZero()
}

// CursorStore persists per-(repository, kind) cursor state across github_read calls.
type CursorStore interface {
	Get(repoKey, kind string) (CursorEntry, bool)
	Put(repoKey, kind string, entry CursorEntry) error
}

// RunCursorStore extends CursorStore with run-scoped staging and commit-on-persist
// semantics (#268):
//   - GetForRun returns the pre-run baseline cursor for (repoKey, kind) so
//     retries within the same run replay the identical delta idempotently.
//   - StageForRun stages a candidate cursor for runID (or commits immediately
//     when runID is empty) without advancing the committed Watermark, Seen set,
//     or LastPersistedAt until CommitRun is invoked.
//   - CommitRun promotes all staged cursors for runID (or a mapped producer
//     loop ID) once the downstream research artifact is persisted.
type RunCursorStore interface {
	CursorStore
	GetForRun(runID, repoKey, kind string) (CursorEntry, bool)
	StageForRun(runID, loopID, repoKey, kind string, next CursorEntry, hasNewRecords bool) error
	CommitRun(runOrLoopID string, committedAt time.Time) error
}

// VersionDigest computes a deterministic version token for a retrieved GitHub
// object combining its activity timestamp and a SHA-256 prefix of its raw body.
func VersionDigest(activityAt time.Time, rawBody string) string {
	sum := sha256.Sum256([]byte(rawBody))
	return fmt.Sprintf("%s#%s", activityAt.UTC().Format(time.RFC3339Nano), hex.EncodeToString(sum[:8]))
}

type stagedCursor struct {
	baseline      CursorEntry
	hasBaseline   bool
	next          CursorEntry
	hasNewRecords bool
}

func applyCursorUpdate(prev CursorEntry, hadPrev bool, next CursorEntry, hasNewRecords bool, persistedAt time.Time) CursorEntry {
	checkedAt := next.LastCheckedAt.UTC()
	if checkedAt.IsZero() {
		checkedAt = persistedAt.UTC()
	}
	if persistedAt.IsZero() {
		persistedAt = checkedAt
	} else {
		persistedAt = persistedAt.UTC()
	}

	// Initial onboarding or incremental delta with new/updated records.
	if !hadPrev || !prev.isBaselined() || hasNewRecords {
		committed := next.clone()
		committed.BaselineEstablished = true
		committed.LastCheckedAt = checkedAt
		committed.LastPersistedAt = persistedAt
		return committed
	}

	// No-change check on an already-baselined stream: advance LastCheckedAt
	// while preserving Watermark, Seen, and LastPersistedAt.
	committed := prev.clone()
	committed.BaselineEstablished = true
	committed.LastCheckedAt = checkedAt
	return committed
}

// MemoryCursorStore is an in-memory thread-safe CursorStore and RunCursorStore.
type MemoryCursorStore struct {
	mu        sync.RWMutex
	entries   map[string]map[string]CursorEntry
	staged    map[string]map[string]map[string]stagedCursor
	loopToRun map[string]string
}

// NewMemoryCursorStore creates an empty MemoryCursorStore.
func NewMemoryCursorStore() *MemoryCursorStore {
	return &MemoryCursorStore{
		entries:   make(map[string]map[string]CursorEntry),
		staged:    make(map[string]map[string]map[string]stagedCursor),
		loopToRun: make(map[string]string),
	}
}

// Get returns the committed CursorEntry for (repoKey, kind) if present.
func (s *MemoryCursorStore) Get(repoKey, kind string) (CursorEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getLocked(repoKey, kind)
}

func (s *MemoryCursorStore) getLocked(repoKey, kind string) (CursorEntry, bool) {
	repoKey = normalizeRepoKey(repoKey)
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

// Put stores the committed CursorEntry for (repoKey, kind).
func (s *MemoryCursorStore) Put(repoKey, kind string, entry CursorEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putLocked(repoKey, kind, entry)
	return nil
}

func (s *MemoryCursorStore) putLocked(repoKey, kind string, entry CursorEntry) {
	repoKey = normalizeRepoKey(repoKey)
	if s.entries[repoKey] == nil {
		s.entries[repoKey] = make(map[string]CursorEntry)
	}
	s.entries[repoKey][kind] = entry.clone()
}

// GetForRun returns the pre-run baseline CursorEntry for (repoKey, kind). When
// runID has already staged an entry for (repoKey, kind) in the current run,
// GetForRun returns the pre-stage baseline snapshot so same-run retries are
// idempotent.
func (s *MemoryCursorStore) GetForRun(runID, repoKey, kind string) (CursorEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	repoKey = normalizeRepoKey(repoKey)
	if runID != "" {
		if byRepo, ok := s.staged[runID]; ok {
			if byKind, ok := byRepo[repoKey]; ok {
				if st, ok := byKind[kind]; ok {
					return st.baseline.clone(), st.hasBaseline
				}
			}
		}
	}
	return s.getLocked(repoKey, kind)
}

// StageForRun stages next under runID when runID is non-empty, or commits
// immediately when runID is empty. For an already-baselined stream where
// hasNewRecords is false, LastCheckedAt is updated on the committed entry
// immediately while preserving Watermark, Seen, and LastPersistedAt.
func (s *MemoryCursorStore) StageForRun(runID, loopID, repoKey, kind string, next CursorEntry, hasNewRecords bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.stageForRunLocked(runID, loopID, repoKey, kind, next, hasNewRecords)
	return nil
}

func (s *MemoryCursorStore) stageForRunLocked(runID, loopID, repoKey, kind string, next CursorEntry, hasNewRecords bool) bool {
	repoKey = normalizeRepoKey(repoKey)
	prev, hadPrev := s.getLocked(repoKey, kind)
	if runID == "" {
		committed := applyCursorUpdate(prev, hadPrev, next, hasNewRecords, next.LastCheckedAt)
		s.putLocked(repoKey, kind, committed)
		return true
	}

	if loopID != "" {
		if s.loopToRun == nil {
			s.loopToRun = make(map[string]string)
		}
		s.loopToRun[loopID] = runID
	}

	if s.staged == nil {
		s.staged = make(map[string]map[string]map[string]stagedCursor)
	}
	if s.staged[runID] == nil {
		s.staged[runID] = make(map[string]map[string]stagedCursor)
	}
	byRepo := s.staged[runID]
	if byRepo[repoKey] == nil {
		byRepo[repoKey] = make(map[string]stagedCursor)
	}

	baseline := prev
	hasBaseline := hadPrev
	if existing, ok := byRepo[repoKey][kind]; ok {
		baseline = existing.baseline
		hasBaseline = existing.hasBaseline
	}

	byRepo[repoKey][kind] = stagedCursor{
		baseline:      baseline.clone(),
		hasBaseline:   hasBaseline,
		next:          next.clone(),
		hasNewRecords: hasNewRecords,
	}

	// If the stream already had a committed baseline, record that a check
	// occurred at next.LastCheckedAt without advancing Watermark, Seen, or
	// LastPersistedAt.
	if hadPrev && prev.isBaselined() && !next.LastCheckedAt.IsZero() {
		updated := prev.clone()
		updated.BaselineEstablished = true
		updated.LastCheckedAt = next.LastCheckedAt.UTC()
		s.putLocked(repoKey, kind, updated)
		return true
	}
	return false
}

// CommitRun promotes all staged cursor entries for runOrLoopID into the
// committed store, stamping LastPersistedAt = committedAt on streams that
// established a baseline or observed new activity.
func (s *MemoryCursorStore) CommitRun(runOrLoopID string, committedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.commitRunLocked(runOrLoopID, committedAt)
	return nil
}

func (s *MemoryCursorStore) commitRunLocked(runOrLoopID string, committedAt time.Time) bool {
	if runOrLoopID == "" {
		return false
	}
	targetRunID := runOrLoopID
	if mapped, ok := s.loopToRun[runOrLoopID]; ok && mapped != "" {
		targetRunID = mapped
	}
	byRepo, ok := s.staged[targetRunID]
	if !ok || len(byRepo) == 0 {
		return false
	}

	for repoKey, byKind := range byRepo {
		for kind, st := range byKind {
			prev, hadPrev := s.getLocked(repoKey, kind)
			committed := applyCursorUpdate(prev, hadPrev, st.next, st.hasNewRecords, committedAt)
			s.putLocked(repoKey, kind, committed)
		}
	}
	delete(s.staged, targetRunID)
	for loopID, rID := range s.loopToRun {
		if rID == targetRunID {
			delete(s.loopToRun, loopID)
		}
	}
	return true
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
		normalized := make(map[string]map[string]CursorEntry, len(loaded))
		for repoKey, byKind := range loaded {
			for kind, entry := range byKind {
				if entry.isBaselined() {
					entry.BaselineEstablished = true
				}
				byKind[kind] = entry
			}
			normalized[normalizeRepoKey(repoKey)] = byKind
		}
		mem.entries = normalized
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
	return s.flushLocked()
}

// GetForRun returns the pre-run baseline CursorEntry for (repoKey, kind).
func (s *FileCursorStore) GetForRun(runID, repoKey, kind string) (CursorEntry, bool) {
	return s.mem.GetForRun(runID, repoKey, kind)
}

// StageForRun stages next under runID (or commits immediately when runID is
// empty) and flushes any committed state change to disk.
func (s *FileCursorStore) StageForRun(runID, loopID, repoKey, kind string, next CursorEntry, hasNewRecords bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem.mu.Lock()
	mutatedCommitted := s.mem.stageForRunLocked(runID, loopID, repoKey, kind, next, hasNewRecords)
	s.mem.mu.Unlock()
	if mutatedCommitted {
		return s.flushLocked()
	}
	return nil
}

// CommitRun promotes staged cursors for runOrLoopID and flushes the updated
// committed store to disk.
func (s *FileCursorStore) CommitRun(runOrLoopID string, committedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem.mu.Lock()
	mutated := s.mem.commitRunLocked(runOrLoopID, committedAt)
	s.mem.mu.Unlock()
	if !mutated {
		return nil
	}
	return s.flushLocked()
}

func (s *FileCursorStore) flushLocked() error {
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
