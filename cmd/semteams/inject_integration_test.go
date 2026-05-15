//go:build integration

package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/persona"
	"github.com/c360studio/semteams/cmd/semteams/testharness"
	"github.com/stretchr/testify/require"
)

// TestInjectRenderedTestHarnessFragment_HappyPath wires real KV-backed
// managers and verifies the synthetic fragment lands with the
// expected ID, Roles, Category, Priority, and rendered content.
func TestInjectRenderedTestHarnessFragment_HappyPath(t *testing.T) {
	tc := natsclient.NewTestClient(t,
		natsclient.WithJetStream(),
		natsclient.WithKV())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	thMgr, err := testharness.NewManager(tc.Client)
	require.NoError(t, err)
	pMgr, err := persona.NewManager(tc.Client)
	require.NoError(t, err)

	// Seed one test harness so the renderer produces the with-entries body.
	require.NoError(t, thMgr.Put(ctx, &testharness.TestHarness{
		Name:                "stub",
		ComposeProfile:      "harness-stub",
		Image:               "scratch",
		SmokeContractSchema: "stub.smoke_contract.v1",
		DomainDescription:   "test stub for integration verification.",
	}))

	injectRenderedTestHarnessFragment(ctx, pMgr, thMgr, slog.Default())

	got, err := pMgr.Get(ctx, "test-harness-catalog.rendered")
	require.NoError(t, err)
	require.Equal(t, "test-harness-catalog.rendered", got.ID)
	require.Equal(t, 0, got.Category, "synthetic fragment should match project baseline (Category=0)")
	require.Equal(t, 45, got.Priority, "synthetic should sort after static 40-harness-catalog within Category=0")
	// Roles list: ADR-041 MVP roster (researcher + reviewer phase suffixes)
	// plus the legacy `researcher` / `research-reviewer` retained for
	// research-iterative configs that have not migrated.
	require.ElementsMatch(t,
		[]string{
			"researcher",
			"research-reviewer",
			"researcher-plan",
			"researcher-gather",
			"researcher-synthesize",
			"researcher-architect",
			"reviewer-research",
			"reviewer-spec",
		},
		got.Roles)
	require.Contains(t, got.Content, "1. `stub`", "rendered body should list the seeded test harness")
}

// TestInjectRenderedTestHarnessFragment_EmptyCatalog covers the
// catalog-miss path: the synthetic fragment must still upsert (with
// the "No test harnesses are currently registered" body) so the LLM
// consistently sees the same fragment-ID structure regardless of
// catalog state.
func TestInjectRenderedTestHarnessFragment_EmptyCatalog(t *testing.T) {
	tc := natsclient.NewTestClient(t,
		natsclient.WithJetStream(),
		natsclient.WithKV())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	thMgr, err := testharness.NewManager(tc.Client)
	require.NoError(t, err)
	pMgr, err := persona.NewManager(tc.Client)
	require.NoError(t, err)

	injectRenderedTestHarnessFragment(ctx, pMgr, thMgr, slog.Default())

	got, err := pMgr.Get(ctx, "test-harness-catalog.rendered")
	require.NoError(t, err)
	require.Contains(t, got.Content, "No test harnesses are currently registered")
	require.NotContains(t, strings.ToLower(got.Content), "image:")
}
