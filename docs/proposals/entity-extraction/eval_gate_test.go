package entityextraction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPhase0CorpusAndFixturesIntegrity(t *testing.T) {
	cp, err := loadCorpus("corpus_v1.json")
	if err != nil {
		t.Fatalf("loadCorpus: %v", err)
	}
	if len(cp.Cases) != 36 {
		t.Fatalf("expected 36 cases, got %d", len(cp.Cases))
	}
	if len(cp.SeedCatalog) != 23 {
		t.Fatalf("expected 23 seed catalog entities, got %d", len(cp.SeedCatalog))
	}
	bandCounts := make(map[string]int)
	for _, c := range cp.Cases {
		bandCounts[c.Band]++
	}
	for _, b := range []string{"B1", "B2", "B3", "B4", "B5", "B6"} {
		if bandCounts[b] != 6 {
			t.Fatalf("band %s count = %d, want 6", b, bandCounts[b])
		}
	}
	cpOK, u16OK, b8OK, err := verifyEmojiSpanFixture(filepath.Join("fixtures", "mercury", "quote", "c5-unicode.json"))
	if err != nil {
		t.Fatalf("verifyEmojiSpanFixture: %v", err)
	}
	if !cpOK || u16OK || b8OK {
		t.Fatalf("emoji span offset check: cpOK=%v u16OK=%v b8OK=%v, want true/false/false", cpOK, u16OK, b8OK)
	}
}

func TestAlgoFloorAndCatalogRegimes(t *testing.T) {
	cp, err := loadCorpus("corpus_v1.json")
	if err != nil {
		t.Fatalf("loadCorpus: %v", err)
	}
	out := evaluateAlgoArm(cp)
	if out.WarmTypeLinkResidualCalls != "6/36" {
		t.Fatalf("warm residual calls = %s, want 6/36", out.WarmTypeLinkResidualCalls)
	}
	if out.PortfolioTypeLinkResidualCalls != "17/36" {
		t.Fatalf("portfolio residual calls = %s, want 17/36", out.PortfolioTypeLinkResidualCalls)
	}
	if out.EmptyTypeLinkResidualCalls != "24/36" {
		t.Fatalf("empty-catalog residual calls = %s, want 24/36", out.EmptyTypeLinkResidualCalls)
	}
	for _, b := range []string{"B1", "B2", "B4", "B6"} {
		bs := out.ByBandMentionCheck[b]
		if bs.TypeOK != "6/6" || bs.LinkOK != "6/6" || bs.SupportOK != "6/6" || bs.FalseMergeRate != 0.0 {
			t.Fatalf("warm band %s failed: %+v", b, bs)
		}
	}
	assertOrWriteJSON(t, filepath.Join("results", "eval_algo_t0_t1.json"), out)
}

func TestDeciderAndHybridArmsMatchResults(t *testing.T) {
	cp, err := loadCorpus("corpus_v1.json")
	if err != nil {
		t.Fatalf("loadCorpus: %v", err)
	}
	for _, backend := range []string{"mercury", "semselect"} {
		fwdOut, err := evaluateModelArm(cp, "eval_cache", backend, "forward")
		if err != nil {
			t.Fatalf("evaluateModelArm %s/forward: %v", backend, err)
		}
		revOut, err := evaluateModelArm(cp, "eval_cache", backend, "reverse")
		if err != nil {
			t.Fatalf("evaluateModelArm %s/reverse: %v", backend, err)
		}
		assertOrWriteJSON(t, filepath.Join("results", "eval_"+backend+"_forward.json"), fwdOut)
		assertOrWriteJSON(t, filepath.Join("results", "eval_"+backend+"_reverse.json"), revOut)

		hFwd := evaluateHybridArm(cp, fwdOut, backend, "forward")
		hRev := evaluateHybridArm(cp, revOut, backend, "reverse")
		assertOrWriteJSON(t, filepath.Join("results", "eval_hybrid_"+backend+"_forward.json"), hFwd)
		assertOrWriteJSON(t, filepath.Join("results", "eval_hybrid_"+backend+"_reverse.json"), hRev)

		agreeOut := evaluateAgreeArm(cp, fwdOut, revOut, backend)
		assertOrWriteJSON(t, filepath.Join("results", "eval_hybrid_agree_"+backend+".json"), agreeOut)
		verifyAgreeAndContradictionInvariants(t, backend, hFwd, hRev, agreeOut)
	}
}

func verifyAgreeAndContradictionInvariants(
	t *testing.T,
	backend string,
	hFwd, hRev hybridEvalOutput,
	agreeOut agreeEvalOutput,
) {
	t.Helper()
	for _, b := range []string{"B1", "B2", "B3", "B4", "B5", "B6"} {
		if hFwd.ByBandContradictionFlag[b].SupportOK != "6/6" || hFwd.ByBandContradictionFlag[b].SupportWrongConfident != 0.0 {
			t.Fatalf("%s/forward contradiction flag failed on %s: %+v", backend, b, hFwd.ByBandContradictionFlag[b])
		}
		if hRev.ByBandContradictionFlag[b].SupportOK != "6/6" || hRev.ByBandContradictionFlag[b].SupportWrongConfident != 0.0 {
			t.Fatalf("%s/reverse contradiction flag failed on %s: %+v", backend, b, hRev.ByBandContradictionFlag[b])
		}
		ab := agreeOut.ByBandStrictBothAboveFloor[b]
		if ab.TypeWrongConfident != 0.0 || ab.FalseMergeRate != 0.0 || ab.SupportWrongConfident != 0.0 {
			t.Fatalf("%s agree arm had non-zero wrong_confident/false_merge on %s: %+v", backend, b, ab)
		}
	}
}

func TestGateSensitivity(t *testing.T) {
	cp, err := loadCorpus("corpus_v1.json")
	if err != nil {
		t.Fatalf("loadCorpus: %v", err)
	}
	out := evaluateAlgoArm(cp)
	mutRows := make([]evalRow, len(out.Rows))
	copy(mutRows, out.Rows)
	// 1. Inject a false catalog merge on B3 and verify FalseMergeRate > 0
	for i := range mutRows {
		if mutRows[i].Case == "b3-04-person-novel" {
			mutRows[i].Link = "entity:person:francois-chollet"
		}
	}
	mutSummary := gradeArm(cp.Cases, mutRows, "support_mention", "mention")
	if mutSummary["B3"].FalseMergeRate == 0.0 {
		t.Fatalf("sensitivity failure: injected false merge was not detected")
	}
	// 2. Verify agreeStrict catches option-order disagreement (b3-02 organization vs software)
	if agreeStrict("organization", "software") != "unsure" {
		t.Fatalf("sensitivity failure: agreeStrict did not abstain on disagreeing option orders")
	}
}

func assertOrWriteJSON(t *testing.T, path string, val any) {
	t.Helper()
	gotBytes, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent %s: %v", path, err)
	}
	gotBytes = append(gotBytes, '\n')
	if os.Getenv("GENERATE_RESULTS") == "1" {
		if err := os.WriteFile(path, gotBytes, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
		return
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Fatalf("result drift in %s (run with GENERATE_RESULTS=1 to regenerate)", path)
	}
}
