package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestChainStallHandlerOrder_LastInSlice pins chainstall's position in
// the chain.CompletionHandler slice instantiated by
// cmd/semteams/main.go's startChainMilestoneSubscribers. chainstall
// reads chain-entity triples written by sibling handlers
// (phasevalidator, specModeGate, qaModeGate, recoverycounter,
// NeedsReviewStamper) on the same agent.complete event; the synchronous
// graph.mutation.triple.add + graph.query.entity request/reply round-trips
// give within-event read-after-sibling-write consistency ONLY when the
// sibling writes complete before chainstall reads — which only holds if
// chainstall is the LAST handler in the slice.
//
// A future maintainer reordering the slice (or inserting a new handler
// after chainstall) would silently break the contract; cmd/semteams/chainstall/doc.go
// §"Within-event read-after-sibling-write coupling" documents this as a
// hard requirement, this test enforces it.
func TestChainStallHandlerOrder_LastInSlice(t *testing.T) {
	repoRoot := repoRootFromTestDir(t)
	body, err := os.ReadFile(filepath.Join(repoRoot, "cmd/semteams/main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	// Match the chain.NewCompletionSubscriber([]chain.CompletionHandler{...})
	// literal. Captures the slice body (identifier list separated by
	// commas/newlines) for inspection.
	sliceRe := regexp.MustCompile(`(?s)chain\.NewCompletionSubscriber\(\s*\[\]chain\.CompletionHandler\{([^}]*)\}`)
	m := sliceRe.FindStringSubmatch(string(body))
	if len(m) < 2 {
		t.Fatal("could not locate chain.NewCompletionSubscriber([]chain.CompletionHandler{…}) literal in main.go — refactor changed the wiring shape and this contract test needs updating")
	}
	sliceBody := m[1]

	// Extract the identifier list. Each handler entry is on its own line
	// or comma-separated; ignore comments and whitespace.
	identRe := regexp.MustCompile(`(?m)^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*,`)
	identMatches := identRe.FindAllStringSubmatch(sliceBody, -1)
	if len(identMatches) == 0 {
		t.Fatal("found empty handler slice — main.go wiring degenerate")
	}

	// The last identifier with a trailing comma is the last handler. The
	// final entry MAY or MAY NOT have a trailing comma — handle both by
	// tolerating one extra ident after the regex captures.
	idents := make([]string, 0, len(identMatches))
	for _, im := range identMatches {
		idents = append(idents, im[1])
	}
	// Also catch a trailing entry without a comma (e.g. `stallSubscriber\n}`).
	trailingRe := regexp.MustCompile(`(?m)^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*$`)
	tm := trailingRe.FindAllStringSubmatch(sliceBody, -1)
	for _, t := range tm {
		idents = append(idents, t[1])
	}
	if len(idents) == 0 {
		t.Fatal("no handler identifiers parsed from slice body")
	}

	last := strings.TrimSpace(idents[len(idents)-1])
	const wantLast = "stallSubscriber"
	if last != wantLast {
		t.Errorf("expected last chain.CompletionHandler to be %q (chainstall), got %q.\nReordering the handler slice breaks chainstall's within-event read-after-sibling-write contract; see cmd/semteams/chainstall/doc.go §\"Within-event read-after-sibling-write coupling\". If this reordering is intentional, update cmd/semteams/chainstall/doc.go and adjust this test together.\nParsed slice order: %v", wantLast, last, idents)
	}
}
