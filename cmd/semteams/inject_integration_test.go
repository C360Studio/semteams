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
	"github.com/c360studio/semteams/cmd/semteams/harness"
	"github.com/stretchr/testify/require"
)

// TestInjectRenderedHarnessFragment_HappyPath wires real KV-backed
// managers and verifies the synthetic fragment lands with the
// expected ID, Roles, Category, Priority, and rendered content.
func TestInjectRenderedHarnessFragment_HappyPath(t *testing.T) {
	tc := natsclient.NewTestClient(t,
		natsclient.WithJetStream(),
		natsclient.WithKV())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hMgr, err := harness.NewManager(tc.Client)
	require.NoError(t, err)
	pMgr, err := persona.NewManager(tc.Client)
	require.NoError(t, err)

	// Seed one harness so the renderer produces the with-entries body.
	require.NoError(t, hMgr.Put(ctx, &harness.Harness{
		Name:                "stub",
		ComposeProfile:      "harness-stub",
		Image:               "scratch",
		SmokeContractSchema: "stub.smoke_contract.v1",
		DomainDescription:   "test stub for integration verification.",
	}))

	injectRenderedHarnessFragment(ctx, pMgr, hMgr, slog.Default())

	got, err := pMgr.Get(ctx, "harness-catalog.rendered")
	require.NoError(t, err)
	require.Equal(t, "harness-catalog.rendered", got.ID)
	require.Equal(t, 0, got.Category, "synthetic fragment should match project baseline (Category=0)")
	require.Equal(t, 45, got.Priority, "synthetic should sort after static 40-harness-catalog within Category=0")
	require.ElementsMatch(t, []string{"researcher", "researcher-with-source-acquisition"}, got.Roles)
	require.Contains(t, got.Content, "1. `stub`", "rendered body should list the seeded harness")
}

// TestInjectRenderedHarnessFragment_EmptyCatalog covers the
// catalog-miss path: the synthetic fragment must still upsert (with
// the "no harnesses registered" body) so the LLM consistently sees
// the same fragment-ID structure regardless of catalog state.
func TestInjectRenderedHarnessFragment_EmptyCatalog(t *testing.T) {
	tc := natsclient.NewTestClient(t,
		natsclient.WithJetStream(),
		natsclient.WithKV())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hMgr, err := harness.NewManager(tc.Client)
	require.NoError(t, err)
	pMgr, err := persona.NewManager(tc.Client)
	require.NoError(t, err)

	injectRenderedHarnessFragment(ctx, pMgr, hMgr, slog.Default())

	got, err := pMgr.Get(ctx, "harness-catalog.rendered")
	require.NoError(t, err)
	require.Contains(t, got.Content, "No harnesses are currently registered")
	require.NotContains(t, strings.ToLower(got.Content), "image:")
}
