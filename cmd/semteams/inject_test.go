package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/c360studio/semstreams/persona"
	"github.com/c360studio/semteams/cmd/semteams/testharness"
)

// TestInjectRenderedTestHarnessFragment_NilManagers verifies the nil
// short-circuit. No NATS dependency — both managers are concrete
// pointer types so passing nil is a typed-nil that the explicit
// `== nil` checks catch before any List/Upsert call.
func TestInjectRenderedTestHarnessFragment_NilManagers(_ *testing.T) {
	logger := slog.Default()
	ctx := context.Background()

	// All four nil combinations — should never panic, never log a
	// warning beyond the harmless skip path.
	injectRenderedTestHarnessFragment(ctx, nil, nil, logger)
	injectRenderedTestHarnessFragment(ctx, nil, &testharness.Manager{}, logger)
	injectRenderedTestHarnessFragment(ctx, &persona.Manager{}, nil, logger)
	// (*Manager{}, *Manager{}) would dereference into KV ops — covered
	// by the integration test, not here.
}
