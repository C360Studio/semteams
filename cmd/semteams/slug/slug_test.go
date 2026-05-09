package slug

import (
	"strings"
	"testing"
	"time"
)

var fixedDate = time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

func TestDeriveDated_TitleOnly(t *testing.T) {
	got := DeriveDated("OSH Meshtastic Driver", "", "", fixedDate)
	want := "2026-05-08-osh-meshtastic-driver"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveDated_TitleTrimmed(t *testing.T) {
	got := DeriveDated("  Hello World  ", "", "", fixedDate)
	want := "2026-05-08-hello-world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveDated_TitleWithSpecialChars(t *testing.T) {
	got := DeriveDated("OSH/Meshtastic: driver_v2!", "", "", fixedDate)
	want := "2026-05-08-osh-meshtastic-driver-v2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveDated_TitleAllNonASCII_NoFallback_TrailingDashSignal(t *testing.T) {
	// emit_dev_via_spec_artifact's contract: empty fallbackPrefix means
	// "reject degenerate input." All-non-ASCII title produces a slug
	// ending in "-" so caller's HasSuffix("-") check fires.
	got := DeriveDated("日本語タイトル", "", "", fixedDate)
	if !strings.HasSuffix(got, "-") {
		t.Errorf("got %q, want trailing-dash signal for degenerate non-ASCII title with no fallback", got)
	}
}

func TestDeriveDated_TitleEmpty_NoFallback_TrailingDashSignal(t *testing.T) {
	got := DeriveDated("", "", "", fixedDate)
	if !strings.HasSuffix(got, "-") {
		t.Errorf("got %q, want trailing-dash signal for empty title with no fallback", got)
	}
}

func TestDeriveDated_TitleEmpty_LoopIDFallback(t *testing.T) {
	got := DeriveDated("", "loop-research-abc123", "research", fixedDate)
	want := "2026-05-08-research-loop-res"
	if got != want {
		t.Errorf("got %q, want %q (research- + first 8 chars of loopID)", got, want)
	}
}

func TestDeriveDated_TitleEmpty_LoopIDShorterThan8(t *testing.T) {
	got := DeriveDated("", "abc", "plan", fixedDate)
	want := "2026-05-08-plan-abc"
	if got != want {
		t.Errorf("got %q, want %q (plan- + full short loopID)", got, want)
	}
}

func TestDeriveDated_TitleEmpty_PrefixOnly(t *testing.T) {
	// fallbackPrefix supplied but loopID is empty (rare — defensive).
	got := DeriveDated("", "", "consensus", fixedDate)
	want := "2026-05-08-consensus"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveDated_TitleAllNonASCII_WithFallback(t *testing.T) {
	// All-non-ASCII title is NOT empty after trim+lowercase, so the
	// loopID-embed fallback path doesn't fire; nonAlnum normalisation
	// reduces the slug to empty, then the final defensive fallback
	// returns just the prefix. Two non-ASCII titles on the same day
	// would collide on the same slug; in practice persona contracts
	// supply ASCII titles. Preserves pre-dedupe behaviour from each
	// emit-tool's local deriveSlug.
	got := DeriveDated("日本語", "loop-x123", "research", fixedDate)
	want := "2026-05-08-research"
	if got != want {
		t.Errorf("got %q, want %q (degenerate title falls back to prefix-only)", got, want)
	}
}

func TestDeriveDated_PathologicalSlugFallsBackToPrefix(t *testing.T) {
	// Title produces empty slug after normalisation but prefix is set:
	// final defense returns just "<date>-<prefix>".
	got := DeriveDated("///", "", "plan", fixedDate)
	want := "2026-05-08-plan"
	if got != want {
		t.Errorf("got %q, want %q (slug-empty fallback to prefix)", got, want)
	}
}

func TestDeriveDated_DateInUTC(t *testing.T) {
	// Late-evening Pacific time → next day UTC. The slug's date stamp
	// is t.Format which uses t's location; helper relies on caller to
	// pass UTC if UTC is wanted. Document by example: a non-UTC time
	// stamps the local date.
	pacific, _ := time.LoadLocation("America/Los_Angeles")
	t1 := time.Date(2026, 5, 7, 22, 0, 0, 0, pacific) // 2026-05-08 05:00 UTC
	got := DeriveDated("Plan", "", "plan", t1)
	want := "2026-05-07-plan"
	if got != want {
		t.Errorf("got %q, want %q (slug uses t's location, not UTC; caller controls)", got, want)
	}
}

func TestStemFromPath_TrailingKindStripped(t *testing.T) {
	got := StemFromPath("docs/research/2026-05-09-osh-meshtastic-driver-research.md", "research")
	want := "2026-05-09-osh-meshtastic-driver"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStemFromPath_AbsolutePath(t *testing.T) {
	got := StemFromPath("/var/build/docs/research/2026-05-09-osh-meshtastic-driver-research.md", "research")
	want := "2026-05-09-osh-meshtastic-driver"
	if got != want {
		t.Errorf("got %q, want %q (absolute path basename is stripped)", got, want)
	}
}

func TestStemFromPath_BasenameOnly(t *testing.T) {
	got := StemFromPath("2026-05-09-osh-meshtastic-driver-research.md", "research")
	want := "2026-05-09-osh-meshtastic-driver"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStemFromPath_EmptyKindSuffix(t *testing.T) {
	// kindSuffix=="" disables suffix-trim; useful when the caller stores
	// the full slug (no kind word) and just wants basename + extension
	// stripped.
	got := StemFromPath("docs/research/2026-05-09-osh-driver.md", "")
	want := "2026-05-09-osh-driver"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStemFromPath_KindSuffixAbsentInBasename(t *testing.T) {
	// Researcher path doesn't end with the expected kind word: return
	// basename minus extension. Stem will not be slug-clean but is
	// preferable to returning empty (no chain.slug.stem stamping is
	// strictly worse than a slightly-off stem).
	got := StemFromPath("docs/research/2026-05-09-osh-driver.md", "research")
	want := "2026-05-09-osh-driver"
	if got != want {
		t.Errorf("got %q, want %q (no -research suffix to strip; basename returned)", got, want)
	}
}

func TestStemFromPath_EmptyPath(t *testing.T) {
	if got := StemFromPath("", "research"); got != "" {
		t.Errorf("got %q, want empty for empty path", got)
	}
}

// TestDeriveDated_DeterministicOnRetry pins the idempotency
// invariant emit-tools rely on: same (title, t.Date) → same slug,
// so a re-emission overwrites the prior file rather than creating
// a fresh one. Smoke regression here would re-introduce the
// "two .md files for one chain" bug Phase C1's overwrite test
// catches downstream.
func TestDeriveDated_DeterministicOnRetry(t *testing.T) {
	t1 := time.Date(2026, 5, 8, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 8, 17, 30, 0, 0, time.UTC)
	got1 := DeriveDated("Stable Title", "loop-abc", "research", t1)
	got2 := DeriveDated("Stable Title", "loop-abc", "research", t2)
	if got1 != got2 {
		t.Errorf("same title + same date should produce same slug; got %q vs %q", got1, got2)
	}
}
