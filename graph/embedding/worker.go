package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/storage"
)

// defaultMaxSourceTextLen is the safety cap for streaming content reads when
// maxSourceTextLen is 0 (unconfigured). Prevents unbounded memory allocation
// for very large stored content.
const defaultMaxSourceTextLen = 8000

// isExpectedShutdownError returns true if the error is expected during component shutdown.
// These include subscription cleanup errors and consumer not found errors which occur
// when NATS resources are cleaned up before or during Stop().
func isExpectedShutdownError(err error) bool {
	if errors.Is(err, nats.ErrBadSubscription) {
		return true
	}
	if errors.Is(err, jetstream.ErrConsumerNotFound) {
		return true
	}
	// Also check error string for cases where errors.Is doesn't match
	errStr := err.Error()
	return strings.Contains(errStr, "invalid subscription") ||
		strings.Contains(errStr, "consumer not found")
}

// GeneratedCallback is called when an embedding is successfully generated.
// The callback receives the entity ID and the generated embedding vector.
type GeneratedCallback func(entityID string, embedding []float32)

// WorkerMetrics provides metrics callbacks for embedding worker operations.
// This allows the worker to report metrics without direct dependency on prometheus.
type WorkerMetrics interface {
	// IncDedupHits increments the deduplication hits counter
	IncDedupHits()
	// IncFailed increments the failed embeddings counter
	IncFailed()
	// SetPending sets the current pending embeddings gauge
	SetPending(count float64)
}

// Worker processes pending embedding requests asynchronously
type Worker struct {
	mu sync.RWMutex

	// Dependencies
	storage  *Storage
	embedder Embedder // HTTP or BM25 embedder

	// KV watching
	indexBucket jetstream.KeyValue
	watcher     jetstream.KeyWatcher

	// Content store for fetching body text from ObjectStore via streaming
	contentStore storage.StreamableStore

	// Callbacks
	onGenerated GeneratedCallback // Called when embedding is generated

	// Metrics
	metrics WorkerMetrics // Optional metrics reporter

	// State
	started  bool
	stopping bool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// Configuration
	workers          int // Number of concurrent workers
	maxSourceTextLen int // Max chars for source text (0 = unlimited)

	// Logger
	logger *slog.Logger
}

// NewWorker creates a new async embedding worker
func NewWorker(
	storage *Storage,
	embedder Embedder,
	indexBucket jetstream.KeyValue,
	logger *slog.Logger,
) *Worker {
	if logger == nil {
		logger = slog.Default()
	}

	return &Worker{
		storage:     storage,
		embedder:    embedder,
		indexBucket: indexBucket,
		workers:     5, // Default concurrent workers
		logger:      logger,
	}
}

// WithWorkers sets the number of concurrent workers
func (w *Worker) WithWorkers(n int) *Worker {
	w.workers = n
	return w
}

// WithContentStore sets the content store for streaming body text retrieval.
// When set, the worker can fetch raw content from storage for records
// that have StorageRef instead of SourceText.
func (w *Worker) WithContentStore(store storage.StreamableStore) *Worker {
	w.contentStore = store
	return w
}

// WithMaxSourceTextLen sets the maximum characters for source text used in
// embedding generation. Text beyond this limit is truncated at a word boundary.
// Default: 0 (unlimited). Recommended: 4000 for BM25, 8000 for neural.
func (w *Worker) WithMaxSourceTextLen(n int) *Worker {
	w.maxSourceTextLen = n
	return w
}

// WithOnGenerated sets a callback that is invoked when an embedding is generated.
// Use this to populate caches or trigger downstream processing.
func (w *Worker) WithOnGenerated(cb GeneratedCallback) *Worker {
	w.onGenerated = cb
	return w
}

// WithMetrics sets the metrics reporter for observability.
func (w *Worker) WithMetrics(m WorkerMetrics) *Worker {
	w.metrics = m
	return w
}

// Start begins watching for pending embeddings and processing them
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return fmt.Errorf("embedding worker already started")
	}

	// Create context for the worker
	w.ctx, w.cancel = context.WithCancel(ctx)

	// Start KV watcher for EMBEDDING_INDEX
	watcher, err := w.indexBucket.WatchAll(w.ctx)
	if err != nil {
		w.cancel()
		return errs.WrapTransient(err, "Worker", "Start", "failed to create KV watcher")
	}
	w.watcher = watcher

	// Start worker goroutines
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go func(workerID int) {
			defer w.wg.Done()
			w.processEmbeddings(workerID)
		}(i)
	}

	w.started = true
	w.logger.Info("Embedding worker started", "workers", w.workers)
	return nil
}

// Stop stops the embedding worker gracefully
func (w *Worker) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	w.stopping = true

	// Cancel context to signal all goroutines to stop
	if w.cancel != nil {
		w.cancel()
	}

	// Stop the watcher
	if w.watcher != nil {
		if err := w.watcher.Stop(); err != nil {
			// Expected errors during shutdown: subscription already cleaned up or consumer deleted
			if !isExpectedShutdownError(err) {
				w.logger.Warn("KV watcher stop error", "error", err)
			}
		}
	}

	// Wait for all goroutines to finish
	w.wg.Wait()

	w.started = false
	w.logger.Info("Embedding worker stopped")
	return nil
}

// processEmbeddings watches for KV changes and processes pending embeddings
func (w *Worker) processEmbeddings(workerID int) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("Embedding worker panic recovered", "worker_id", workerID, "panic", r)
		}
	}()

	w.logger.Debug("Embedding worker goroutine started", "worker_id", workerID)

	for {
		select {
		case <-w.ctx.Done():
			w.logger.Debug("Embedding worker context cancelled", "worker_id", workerID)
			return

		case entry, ok := <-w.watcher.Updates():
			if !ok {
				w.logger.Debug("KV watcher updates channel closed", "worker_id", workerID)
				return
			}

			if entry == nil {
				continue
			}

			// Process if this is a new pending record or update to existing pending
			if entry.Operation() == jetstream.KeyValuePut {
				w.handleKVEntry(entry, workerID)
			}
		}
	}
}

// handleKVEntry processes a KV entry to check if it needs embedding generation
func (w *Worker) handleKVEntry(entry jetstream.KeyValueEntry, workerID int) {
	// Parse the record to check status
	var record Record
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		w.logger.Warn("Failed to unmarshal embedding record", "key", entry.Key(), "error", err)
		return
	}

	// Only process pending records
	if record.Status != StatusPending {
		return
	}

	entityID := entry.Key()
	w.logger.Debug("Processing pending embedding", "worker_id", workerID, "entity_id", entityID)

	// Get source text - either from record or from ObjectStore via StorageRef
	sourceText, err := w.getSourceText(&record)
	if err != nil {
		w.logger.Error("Failed to get source text", "entity_id", entityID, "error", err)
		w.markFailed(entityID, fmt.Sprintf("text extraction failed: %v", err))
		return
	}

	if sourceText == "" {
		w.logger.Debug("No source text found, skipping embedding", "entity_id", entityID)
		// Not a failure - just nothing to embed. Remove pending record.
		if err := w.storage.DeleteEmbedding(w.ctx, entityID); err != nil {
			w.logger.Debug("Failed to delete pending record for entity with no text", "entity_id", entityID, "error", err)
		}
		return
	}

	// Get or generate embedding vector
	vector, err := w.getOrGenerateEmbedding(entityID, sourceText, record.ContentHash)
	if err != nil {
		return // Error already logged and marked as failed
	}

	// Save and notify
	w.saveAndNotify(entityID, vector)
}

// getOrGenerateEmbedding returns an existing embedding via dedup or generates a new one.
func (w *Worker) getOrGenerateEmbedding(entityID, sourceText, contentHash string) ([]float32, error) {
	// Check deduplication first
	dedupRecord, err := w.storage.GetByContentHash(w.ctx, contentHash)
	if err != nil {
		w.logger.Error("Failed to check dedup", "entity_id", entityID, "error", err)
		w.markFailed(entityID, fmt.Sprintf("dedup check failed: %v", err))
		return nil, err
	}

	if dedupRecord != nil {
		w.logger.Debug("Deduplicating embedding", "entity_id", entityID, "content_hash", contentHash)
		if w.metrics != nil {
			w.metrics.IncDedupHits()
		}
		return dedupRecord.Vector, nil
	}

	// Generate new embedding
	w.logger.Debug("Generating new embedding", "entity_id", entityID)
	vectors, err := w.embedder.Generate(w.ctx, []string{sourceText})
	if err != nil {
		w.logger.Error("Failed to generate embedding", "entity_id", entityID, "error", err)
		w.markFailed(entityID, fmt.Sprintf("generation failed: %v", err))
		return nil, err
	}

	if len(vectors) == 0 {
		w.logger.Error("No embedding generated", "entity_id", entityID)
		w.markFailed(entityID, "no embedding returned")
		return nil, fmt.Errorf("no embedding returned")
	}

	vector := vectors[0]

	// Save to dedup bucket
	if err := w.storage.SaveDedup(w.ctx, contentHash, vector, entityID); err != nil {
		w.logger.Warn("Failed to save dedup record", "entity_id", entityID, "error", err)
		// Continue anyway - not critical
	}

	return vector, nil
}

// saveAndNotify saves the generated embedding and notifies callback.
func (w *Worker) saveAndNotify(entityID string, vector []float32) {
	dimensions := len(vector)
	model := w.embedder.Model()
	if err := w.storage.SaveGenerated(w.ctx, entityID, vector, model, dimensions); err != nil {
		w.logger.Error("Failed to save generated embedding", "entity_id", entityID, "error", err)
		w.markFailed(entityID, fmt.Sprintf("save failed: %v", err))
		return
	}

	w.logger.Debug("Embedding generated successfully", "entity_id", entityID, "dimensions", dimensions)

	if w.onGenerated != nil {
		w.onGenerated(entityID, vector)
	}
}

// getSourceText extracts text from the record.
// For legacy records, uses SourceText directly.
// For ContentStorable records (with StorageRef), fetches from ObjectStore.
func (w *Worker) getSourceText(record *Record) (string, error) {
	var text string

	// Legacy path: use SourceText if available
	if record.SourceText != "" {
		text = record.SourceText
	} else if record.StorageRef != nil {
		// Streaming path: read raw content from store
		var err error
		text, err = w.fetchTextFromStorage(record.StorageRef)
		if err != nil {
			return "", err
		}
	}

	// Truncate if configured
	if w.maxSourceTextLen > 0 && len(text) > w.maxSourceTextLen {
		text = truncateAtWord(text, w.maxSourceTextLen)
	}

	return text, nil
}

// truncateAtWord truncates text at the last word boundary before maxLen.
func truncateAtWord(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	// Find last space before maxLen
	truncated := text[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 { // Only use word boundary if it's not too far back
		return truncated[:lastSpace]
	}
	return truncated
}

// fetchTextFromStorage streams raw content from the store, reading only up to
// maxSourceTextLen bytes. ObjectStore holds raw bytes (plain text, not JSON-wrapped).
// Triples carry metadata (mime type, hash); the store is format-agnostic.
func (w *Worker) fetchTextFromStorage(ref *StorageRef) (string, error) {
	if w.contentStore == nil {
		return "", fmt.Errorf("content store not configured")
	}

	reader, err := w.contentStore.Open(w.ctx, ref.Key)
	if err != nil {
		return "", fmt.Errorf("failed to open content: %w", err)
	}
	defer reader.Close()

	// Read only what we need — no full memory load
	limit := w.maxSourceTextLen
	if limit <= 0 {
		limit = defaultMaxSourceTextLen
	}
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)))
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	// Detect likely JSON-wrapped content (StoredContent envelope).
	// Raw text is expected — if it starts with '{', someone probably used
	// StoreContent() instead of Put(). Embeddings will include JSON noise.
	if len(data) > 0 && data[0] == '{' {
		w.logger.Debug("stored content appears JSON-wrapped, expected raw text",
			slog.String("key", ref.Key),
			slog.String("hint", "use Put() for raw body text, not StoreContent()"))
	}

	return string(data), nil
}

// markFailed marks an embedding as failed
func (w *Worker) markFailed(entityID, errorMsg string) {
	// Don't count context cancellation (shutdown) as a failure
	if strings.Contains(errorMsg, "context canceled") {
		w.logger.Debug("Skipping failure metric for context cancellation", "entity_id", entityID)
		return
	}

	if err := w.storage.SaveFailed(w.ctx, entityID, errorMsg); err != nil {
		w.logger.Error("Failed to mark embedding as failed", "entity_id", entityID, "error", err)
	}
	if w.metrics != nil {
		w.metrics.IncFailed()
	}
}
