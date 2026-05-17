package flowtemplates

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semstreams/flowtemplate"
	"github.com/c360studio/semstreams/natsclient"
)

// Manager is the subset of flowtemplate.Manager the loader needs.
// Declared here so tests can substitute in-memory fakes without
// requiring NATS. Matches executors.FlowTemplateManager structurally
// but scoped down to the three methods the loader uses.
type Manager interface {
	Create(ctx context.Context, t *flowtemplate.Template) error
	Update(ctx context.Context, t *flowtemplate.Template) error
	Get(ctx context.Context, id string) (*flowtemplate.Template, error)
}

// LoadFromDirectory walks <root>/*.json and upserts each file into the
// flow-template manager. Idempotent re-seed: existing template IDs
// are updated (last-writer-wins), new IDs are created.
//
// Source-of-truth semantics mirror persona.LoadFromDirectory: files on
// disk are the durable source of truth. Each call overwrites any KV
// entry whose template ID matches a file here, including entries
// written at runtime via create_flow_template / update_flow_template.
// Runtime tool edits are ephemeral and reset to file state on the
// next restart. Call LoadFromDirectory at startup, before the tool
// registry starts processing tool calls.
//
// Missing directories and read errors are logged as warnings but do
// not fail boot. Templates that fail Validate() are skipped with a
// warning — boot stays alive. The first upsert error encountered is
// returned to the caller for visibility; subsequent errors are
// logged but don't abort the walk.
func LoadFromDirectory(ctx context.Context, root string, mgr Manager, logger *slog.Logger) error {
	if mgr == nil {
		return nil
	}
	return walkTemplates(ctx, root, mgr, logger)
}

// walkTemplates is the testable core of LoadFromDirectory. Accepting the
// Manager interface lets unit tests inject in-memory fakes without NATS.
func walkTemplates(ctx context.Context, root string, mgr Manager, logger *slog.Logger) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		logger.Warn("flow-template loader: could not resolve root path",
			slog.String("root", root),
			slog.Any("error", err))
		return nil
	}

	if _, statErr := os.Stat(absRoot); os.IsNotExist(statErr) {
		logger.Warn("flow-template loader: directory does not exist, skipping",
			slog.String("root", absRoot))
		return nil
	}

	var (
		loaded         int
		firstUpsertErr error
	)

	walkErr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErrIn error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if walkErrIn != nil {
			logger.Warn("flow-template loader: walk error",
				slog.String("path", path),
				slog.Any("error", walkErrIn))
			return nil
		}

		if path == absRoot {
			return nil
		}

		// Templates live flat at depth-1; skip any nested directory subtree.
		if d.IsDir() {
			return filepath.SkipDir
		}

		fileName := d.Name()
		if strings.HasPrefix(fileName, ".") {
			return nil
		}
		if !strings.HasSuffix(fileName, ".json") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			logger.Warn("flow-template loader: could not read file, skipping",
				slog.String("path", path),
				slog.Any("error", readErr))
			return nil
		}

		var t flowtemplate.Template
		if unmarshalErr := json.Unmarshal(data, &t); unmarshalErr != nil {
			logger.Warn("flow-template loader: malformed JSON, skipping",
				slog.String("path", path),
				slog.Any("error", unmarshalErr))
			return nil
		}

		if validateErr := t.Validate(); validateErr != nil {
			logger.Warn("flow-template loader: template validation failed, skipping",
				slog.String("path", path),
				slog.String("template_id", t.ID),
				slog.Any("error", validateErr))
			return nil
		}

		if upsertErr := upsert(ctx, mgr, &t); upsertErr != nil {
			logger.Warn("flow-template loader: upsert failed, skipping",
				slog.String("path", path),
				slog.String("template_id", t.ID),
				slog.Any("error", upsertErr))
			if firstUpsertErr == nil {
				firstUpsertErr = upsertErr
			}
			return nil
		}

		logger.Debug("flow-template loader: loaded template",
			slog.String("template_id", t.ID),
			slog.String("path", path),
			slog.Int("bytes", len(data)))

		loaded++
		return nil
	})

	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		logger.Warn("flow-template loader: walk failed",
			slog.String("root", absRoot),
			slog.Any("error", walkErr))
	}

	logFn := logger.Info
	if loaded == 0 {
		logFn = logger.Debug
	}
	logFn("flow-template loader: load complete",
		slog.Int("templates", loaded),
		slog.String("root", absRoot))

	// Context cancellation is the caller's shutdown signal — propagate
	// so they see it. Other walk errors stay warn-only; firstUpsertErr
	// surfaces partial-failure upsert problems for boot-time visibility.
	if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
		return walkErr
	}
	return firstUpsertErr
}

// upsert tries Update if the template exists, falls back to Create
// only on a confirmed "key not found." Two-call dance because
// flowtemplate.Manager does not expose an Upsert primitive (unlike
// persona.Manager). Migration target: upstream
// flowtemplate.Manager.Upsert (file upstream issue when Phase 1
// confirms the gap; see ADR-042).
//
// Error discipline: upstream Get wraps every KV error via
// errs.WrapTransient so the wrap layer alone cannot distinguish
// "missing" from "transient hiccup." natsclient.IsKVNotFoundError
// inspects the error message (substring "key not found") and the
// well-known sentinel ErrKVKeyNotFound, which both survive the
// wrap. Any other Get error surfaces directly — we do NOT
// optimistically Create on transient failures because that would
// produce misleading "already exists" conflicts when the next
// retry succeeds.
//
// Ctx cancellation is rechecked between Get and Create because a
// cancellation that lands after Get returns but before Create
// fires would otherwise still attempt the write.
func upsert(ctx context.Context, mgr Manager, t *flowtemplate.Template) error {
	_, err := mgr.Get(ctx, t.ID)
	if err == nil {
		return mgr.Update(ctx, t)
	}
	if !natsclient.IsKVNotFoundError(err) {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return mgr.Create(ctx, t)
}
