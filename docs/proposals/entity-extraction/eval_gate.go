package entityextraction

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

type cachedEntry struct {
	Request struct {
		State string `json:"state"`
		Model string `json:"model,omitempty"`
	} `json:"request"`
	Status    int             `json:"status"`
	LatencyMS int             `json:"latency_ms"`
	Response  deciderResponse `json:"response"`
}

type deciderResponse struct {
	Answers struct {
		Type struct {
			Choice     string   `json:"choice"`
			Confidence *float64 `json:"confidence"`
		} `json:"type"`
		Link struct {
			Choice     string   `json:"choice"`
			Confidence *float64 `json:"confidence"`
		} `json:"link"`
		Support struct {
			Noul        *float64 `json:"noul"`
			Probability *float64 `json:"probability"`
			PYes        *float64 `json:"p_yes"`
		} `json:"support"`
		Mentions struct {
			Spans map[string][]struct {
				Text       string  `json:"text"`
				Start      int     `json:"start"`
				End        int     `json:"end"`
				Confidence float64 `json:"confidence"`
			} `json:"spans"`
		} `json:"mentions"`
	} `json:"answers"`
	Usage struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

func loadCorpus(path string) (corpus, error) {
	var cp corpus
	raw, err := os.ReadFile(path)
	if err != nil {
		return cp, err
	}
	if err := json.Unmarshal(raw, &cp); err != nil {
		return cp, err
	}
	return cp, nil
}

func loadCachedCase(cacheDir, backend, order string, c corpusCase, th gateThresholds) (evalRow, error) {
	pattern := filepath.Join(cacheDir, backend, order, c.ID+"_*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return evalRow{}, fmt.Errorf("missing cache file for %s/%s/%s", backend, order, c.ID)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return evalRow{}, err
	}
	var entry cachedEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return evalRow{}, err
	}
	if entry.Request.State != c.Quote {
		return evalRow{}, fmt.Errorf("evidence-only state violation on %s: state != quote", c.ID)
	}
	return parseDeciderResponse(c, entry.Response, entry.LatencyMS, true, backend, th), nil
}

func parseDeciderResponse(
	c corpusCase,
	resp deciderResponse,
	ms int,
	cachedHit bool,
	backend string,
	th gateThresholds,
) evalRow {
	tChoice := resp.Answers.Type.Choice
	if tChoice == "" {
		tChoice = "unsure"
	}
	lChoice := resp.Answers.Link.Choice
	if lChoice == "" {
		lChoice = "unsure"
	}
	tConf := resp.Answers.Type.Confidence
	lConf := resp.Answers.Link.Confidence
	pYes := firstNonNil(resp.Answers.Support.Noul, resp.Answers.Support.Probability, resp.Answers.Support.PYes)

	confThresh, contraThresh := thresholdsForBackend(backend, th)
	typePred := "unsure"
	if tConf != nil && *tConf >= confThresh {
		typePred = tChoice
	}
	linkPred := "unsure"
	if lConf != nil && *lConf >= confThresh {
		linkPred = lChoice
	}
	suppPred, contraFlag, suppContra := classifySupport(pYes, th.SupportThresholdYes, th.SupportThresholdNo, contraThresh)
	hit := cachedHit
	return evalRow{
		Case:                 c.ID,
		Band:                 c.Band,
		Tier:                 "T2-model",
		Type:                 typePred,
		TypeRaw:              tChoice,
		TypeConf:             tConf,
		Link:                 linkPred,
		LinkRaw:              lChoice,
		LinkConf:             lConf,
		PSupport:             pYes,
		ContradictionFlagged: contraFlag,
		SupportMention:       suppPred,
		SupportStrict:        suppPred,
		SupportContradiction: suppContra,
		InputTokens:          resp.Usage.InputTokens,
		LatencyMS:            ms,
		Cached:               &hit,
	}
}

func firstNonNil(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func thresholdsForBackend(backend string, th gateThresholds) (float64, float64) {
	if backend == "mercury" {
		return th.ConfidenceThresholdMercury, th.ContradictionThresholdMercury
	}
	return th.ConfidenceThresholdSemselect, th.ContradictionThresholdSemsel
}

func classifySupport(pYes *float64, yesFloor, noCeil, contraCeil float64) (any, *bool, any) {
	if pYes == nil {
		return "unsure", nil, "unsure"
	}
	flagged := *pYes <= contraCeil
	suppContra := !flagged
	if *pYes >= yesFloor {
		return true, &flagged, suppContra
	}
	if *pYes <= noCeil {
		return false, &flagged, suppContra
	}
	return "unsure", &flagged, suppContra
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func gradeArm(cases []corpusCase, rows []evalRow, supportField, expectKey string) map[string]bandSummary {
	byID := make(map[string]corpusCase, len(cases))
	for _, c := range cases {
		byID[c.ID] = c
	}
	byBandRows := make(map[string][]evalRow)
	for _, r := range rows {
		byBandRows[r.Band] = append(byBandRows[r.Band], r)
	}
	out := make(map[string]bandSummary, len(byBandRows))
	for band, bRows := range byBandRows {
		out[band] = gradeBand(byID, band, bRows, supportField, expectKey)
	}
	return out
}

func gradeBand(
	byID map[string]corpusCase,
	band string,
	bRows []evalRow,
	supportField, expectKey string,
) bandSummary {
	n := len(bRows)
	var typeCov, typeOK, typeWC, linkCov, linkOK, falseMerge int
	var suppCov, suppOK, suppWC, tokens int
	var latencies []int

	for _, r := range bRows {
		c := byID[r.Case]
		tokens += r.InputTokens
		if r.LatencyMS > 0 {
			latencies = append(latencies, r.LatencyMS)
		}
		sVal := selectSupportValue(r, supportField)
		expS := expectedSupport(c.Expect, expectKey)
		if bVal, isBool := sVal.(bool); isBool {
			suppCov++
			if bVal == expS {
				suppOK++
			} else {
				suppWC++
			}
		}
		if band == "B4" {
			if bVal, isBool := sVal.(bool); isBool {
				typeCov++
				linkCov++
				if !bVal {
					typeOK++
					linkOK++
				} else {
					typeWC++
					falseMerge++
				}
			}
			continue
		}
		if r.Type != "" && r.Type != "unsure" {
			typeCov++
			if r.Type == c.Expect.Type {
				typeOK++
			} else {
				typeWC++
			}
		}
		if r.Link != "" && r.Link != "unsure" {
			linkCov++
			if r.Link == c.Expect.Link {
				linkOK++
			} else if r.Link != "new" {
				falseMerge++
			}
		}
	}
	sort.Ints(latencies)
	medMS := 0
	if len(latencies) > 0 {
		medMS = latencies[len(latencies)/2]
	}
	nf := float64(n)
	return bandSummary{
		Count:                 n,
		TypeCoverage:          round3(float64(typeCov) / nf),
		TypeOK:                fmt.Sprintf("%d/%d", typeOK, n),
		TypeWrongConfident:    round3(float64(typeWC) / nf),
		LinkCoverage:          round3(float64(linkCov) / nf),
		LinkOK:                fmt.Sprintf("%d/%d", linkOK, n),
		FalseMergeRate:        round3(float64(falseMerge) / nf),
		SupportCoverage:       round3(float64(suppCov) / nf),
		SupportOK:             fmt.Sprintf("%d/%d", suppOK, n),
		SupportWrongConfident: round3(float64(suppWC) / nf),
		InputTokens:           tokens,
		MedianMS:              medMS,
	}
}

func selectSupportValue(r evalRow, field string) any {
	switch field {
	case "support_mention":
		return r.SupportMention
	case "support_contradiction":
		if r.SupportContradiction != nil {
			return r.SupportContradiction
		}
		return r.SupportMention
	default:
		return r.SupportStrict
	}
}

func expectedSupport(exp caseExpect, expectKey string) bool {
	switch expectKey {
	case "mention":
		return exp.Mention
	case "not_contradicted":
		return exp.Mention && !strings.HasPrefix(exp.SupportNote, "explicit_contradiction:")
	default:
		return exp.Support
	}
}

func countResidualCalls(rows []evalRow) int {
	calls := 0
	for _, r := range rows {
		if r.Mention != nil && !*r.Mention {
			continue
		}
		if r.Type == "unsure" || r.Link == "unsure" {
			calls++
		}
	}
	return calls
}

func evaluateAlgoArm(cp corpus) algoEvalOutput {
	warmAlgo := newAlgoFloor(cp.SeedCatalog)
	portCatalog := portfolioOnlyCatalog(cp.SeedCatalog)
	portAlgo := newAlgoFloor(portCatalog)
	emptyAlgo := newAlgoFloor(nil)

	warmRows := make([]evalRow, len(cp.Cases))
	portRows := make([]evalRow, len(cp.Cases))
	emptyRows := make([]evalRow, len(cp.Cases))
	for i, c := range cp.Cases {
		warmRows[i] = warmAlgo.evaluateCase(c)
		portRows[i] = portAlgo.evaluateCase(c)
		emptyRows[i] = emptyAlgo.evaluateCase(c)
	}
	n := len(cp.Cases)
	return algoEvalOutput{
		Arm:                            "algo_t0_t1",
		GateThresholds:                 cp.GateThresholds,
		WarmCatalogSize:                len(cp.SeedCatalog),
		PortfolioCatalogSize:           len(portCatalog),
		EmptyCatalogSize:               0,
		WarmTypeLinkResidualCalls:      fmt.Sprintf("%d/%d", countResidualCalls(warmRows), n),
		PortfolioTypeLinkResidualCalls: fmt.Sprintf("%d/%d", countResidualCalls(portRows), n),
		EmptyTypeLinkResidualCalls:     fmt.Sprintf("%d/%d", countResidualCalls(emptyRows), n),
		Rows:                           warmRows,
		RowsPortfolioCatalog:           portRows,
		RowsEmptyCatalog:               emptyRows,
		ByBandMentionCheck:             gradeArm(cp.Cases, warmRows, "support_mention", "mention"),
		ByBandMentionAsRoleSupport:     gradeArm(cp.Cases, warmRows, "support_mention", "support"),
		ByBandStrictRoleSupport:        gradeArm(cp.Cases, warmRows, "support_strict", "support"),
		ByBandPortfolioCatalog:         gradeArm(cp.Cases, portRows, "support_mention", "mention"),
		ByBandEmptyCatalog:             gradeArm(cp.Cases, emptyRows, "support_mention", "mention"),
	}
}

func evaluateModelArm(cp corpus, cacheDir, backend, order string) (modelEvalOutput, error) {
	rows := make([]evalRow, len(cp.Cases))
	for i, c := range cp.Cases {
		r, err := loadCachedCase(cacheDir, backend, order, c, cp.GateThresholds)
		if err != nil {
			return modelEvalOutput{}, err
		}
		rows[i] = r
	}
	return modelEvalOutput{
		Arm:                    backend,
		Order:                  order,
		GateThresholds:         cp.GateThresholds,
		ElapsedS:               0.0,
		Rows:                   rows,
		ByBand:                 gradeArm(cp.Cases, rows, "support_strict", "support"),
		ByBandContradictionFlg: gradeArm(cp.Cases, rows, "support_contradiction", "not_contradicted"),
	}, nil
}

func evaluateHybridArm(cp corpus, modelOut modelEvalOutput, backend, order string) hybridEvalOutput {
	warmAlgo := newAlgoFloor(cp.SeedCatalog)
	portAlgo := newAlgoFloor(portfolioOnlyCatalog(cp.SeedCatalog))
	emptyAlgo := newAlgoFloor(nil)

	modelByID := make(map[string]evalRow, len(modelOut.Rows))
	for _, r := range modelOut.Rows {
		modelByID[r.Case] = r
	}

	hybridRows, cueRows, tlCalls, cueCalls, strictCalls := buildHybridRows(cp.Cases, warmAlgo, modelByID, backend)
	portRows, _, portTLCalls, _, _ := buildHybridRows(cp.Cases, portAlgo, modelByID, backend)
	emptyRows, _, emptyTLCalls, _, _ := buildHybridRows(cp.Cases, emptyAlgo, modelByID, backend)

	var strictTokens, cueTokens int
	for i := range hybridRows {
		strictTokens += hybridRows[i].InputTokens
		cueTokens += cueRows[i].InputTokens
	}
	n := len(cp.Cases)
	return hybridEvalOutput{
		Arm:                           fmt.Sprintf("hybrid_%s_%s", backend, order),
		GateThresholds:                cp.GateThresholds,
		RoutingPolicy:                 "non-oracle: T0 mention gate (suppresses unmentioned claims) + T0/T1 type/link lock (model type/link only when T0/T1 returns unsure)",
		TypeLinkOnlyModelCalls:        fmt.Sprintf("%d/%d", tlCalls, n),
		PortfolioCatalogModelCalls:    fmt.Sprintf("%d/%d", portTLCalls, n),
		EmptyCatalogModelCalls:        fmt.Sprintf("%d/%d", emptyTLCalls, n),
		CueRouterModelCalls:           fmt.Sprintf("%d/%d", cueCalls, n),
		StrictSupportModelCalls:       fmt.Sprintf("%d/%d", strictCalls, n),
		TotalInputTokensStrictSupport: strictTokens,
		TotalInputTokensCueRouter:     cueTokens,
		ByBand:                        gradeArm(cp.Cases, hybridRows, "support_strict", "support"),
		ByBandContradictionFlag:       gradeArm(cp.Cases, hybridRows, "support_contradiction", "not_contradicted"),
		ByBandCueRouterStrictSupport:  gradeArm(cp.Cases, cueRows, "support_strict", "support"),
		ByBandCueRouterContradictMention: gradeArm(
			cp.Cases, cueRows, "support_mention", "not_contradicted",
		),
		ByBandPortfolioCatalog: gradeArm(cp.Cases, portRows, "support_contradiction", "not_contradicted"),
		ByBandEmptyCatalog:     gradeArm(cp.Cases, emptyRows, "support_contradiction", "not_contradicted"),
		Rows:                   hybridRows,
		RowsCueRouter:          cueRows,
		RowsPortfolioCatalog:   portRows,
		RowsEmptyCatalog:       emptyRows,
	}
}

func buildHybridRows(
	cases []corpusCase,
	af *algoFloor,
	modelByID map[string]evalRow,
	backend string,
) ([]evalRow, []evalRow, int, int, int) {
	var hybridRows, cueRows []evalRow
	var tlCalls, cueCalls, strictCalls int

	for _, c := range cases {
		aRow := af.evaluateCase(c)
		mRow := modelByID[c.ID]
		if aRow.Mention != nil && !*aRow.Mention {
			hybridRows = append(hybridRows, aRow)
			cueRows = append(cueRows, aRow)
			continue
		}
		needType := aRow.Type == "unsure"
		needLink := aRow.Link == "unsure"
		if needType || needLink {
			tlCalls++
		}
		strictCalls++
		hType := aRow.Type
		if needType {
			hType = mRow.Type
		}
		hLink := aRow.Link
		if needLink {
			hLink = mRow.Link
		}
		hybridRows = append(hybridRows, evalRow{
			Case:                 c.ID,
			Band:                 c.Band,
			Tier:                 fmt.Sprintf("T0/T1+%s", backend),
			Type:                 hType,
			Link:                 hLink,
			Mention:              aRow.Mention,
			ContradictionFlagged: mRow.ContradictionFlagged,
			SupportMention:       aRow.SupportMention,
			SupportStrict:        mRow.SupportStrict,
			SupportContradiction: mRow.SupportContradiction,
			InputTokens:          mRow.InputTokens,
			LatencyMS:            mRow.LatencyMS,
		})

		hasCue := af.hasContradictionCue(c.Role, c.Quote)
		usedCue := needType || needLink || hasCue
		cueTier := aRow.Tier
		cueTokens, cueMS := 0, 0
		cueSuppMention := aRow.SupportMention
		cueSuppStrict := aRow.SupportStrict
		if usedCue {
			cueCalls++
			cueTier = fmt.Sprintf("T0/T1+%s-cue", backend)
			cueTokens = mRow.InputTokens
			cueMS = mRow.LatencyMS
			cueSuppStrict = mRow.SupportStrict
		}
		if hasCue {
			cueSuppMention = mRow.SupportContradiction
		}
		cueRows = append(cueRows, evalRow{
			Case:                 c.ID,
			Band:                 c.Band,
			Tier:                 cueTier,
			Type:                 hType,
			Link:                 hLink,
			Mention:              aRow.Mention,
			ContradictionFlagged: mRow.ContradictionFlagged,
			SupportMention:       cueSuppMention,
			SupportStrict:        cueSuppStrict,
			SupportContradiction: cueSuppMention,
			InputTokens:          cueTokens,
			LatencyMS:            cueMS,
		})
	}
	return hybridRows, cueRows, tlCalls, cueCalls, strictCalls
}

// evaluateAgreeArm implements Arm 5 (Fable finding A3): non-oracle Gated Hybrid
// with both-orders agreement (forward == reverse) on the residual.
func evaluateAgreeArm(cp corpus, fwdOut, revOut modelEvalOutput, backend string) agreeEvalOutput {
	af := newAlgoFloor(cp.SeedCatalog)
	fwdByID := make(map[string]evalRow, len(fwdOut.Rows))
	revByID := make(map[string]evalRow, len(revOut.Rows))
	for _, r := range fwdOut.Rows {
		fwdByID[r.Case] = r
	}
	for _, r := range revOut.Rows {
		revByID[r.Case] = r
	}
	confFloor, _ := thresholdsForBackend(backend, cp.GateThresholds)
	var strictRows, meanRows []evalRow
	var residualCases, totalRequests, totalTokens int

	for _, c := range cp.Cases {
		aRow := af.evaluateCase(c)
		if aRow.Mention != nil && !*aRow.Mention {
			strictRows = append(strictRows, aRow)
			meanRows = append(meanRows, aRow)
			continue
		}
		fRow, rRow := fwdByID[c.ID], revByID[c.ID]
		sRow, mRow, used := resolveAgreeCase(c, aRow, fRow, rRow, backend, confFloor)
		if used {
			residualCases++
			totalRequests += 2
			totalTokens += sRow.InputTokens
		}
		strictRows = append(strictRows, sRow)
		meanRows = append(meanRows, mRow)
	}
	n := len(cp.Cases)
	return agreeEvalOutput{
		Arm:                  fmt.Sprintf("hybrid_agree_%s", backend),
		GateThresholds:       cp.GateThresholds,
		RoutingPolicy:        "non-oracle: T0 mention gate + T0/T1 lock + both-orders (forward == reverse) agreement on residual unsure cases",
		ResidualCasesQueried: fmt.Sprintf("%d/%d", residualCases, n),
		TotalModelRequests:   fmt.Sprintf("%d", totalRequests),
		TotalInputTokens:     totalTokens,
		ByBandStrictBothAboveFloor: gradeArm(
			cp.Cases, strictRows, "support_contradiction", "not_contradicted",
		),
		ByBandMeanProbAgreement: gradeArm(
			cp.Cases, meanRows, "support_contradiction", "not_contradicted",
		),
		ByBandContradictionFlag: gradeArm(
			cp.Cases, strictRows, "support_contradiction", "not_contradicted",
		),
		Rows: strictRows,
	}
}

func resolveAgreeCase(
	c corpusCase,
	aRow, fRow, rRow evalRow,
	backend string,
	confFloor float64,
) (evalRow, evalRow, bool) {
	needType := aRow.Type == "unsure"
	needLink := aRow.Link == "unsure"
	hasCue := strings.Contains(foldText(c.Quote), "never") ||
		strings.Contains(foldText(c.Quote), "no ") ||
		strings.Contains(foldText(c.Quote), "read-only")
	used := needType || needLink || hasCue

	sType, mType := aRow.Type, aRow.Type
	if needType {
		sType = agreeStrict(fRow.Type, rRow.Type)
		mType = agreeMean(fRow.TypeRaw, rRow.TypeRaw, fRow.TypeConf, rRow.TypeConf, confFloor)
	}
	sLink, mLink := aRow.Link, aRow.Link
	if needLink {
		sLink = agreeStrict(fRow.Link, rRow.Link)
		mLink = agreeMean(fRow.LinkRaw, rRow.LinkRaw, fRow.LinkConf, rRow.LinkConf, confFloor)
	}
	suppContra := aRow.SupportContradiction
	if hasCue && fRow.SupportContradiction == rRow.SupportContradiction {
		suppContra = fRow.SupportContradiction
	}
	toks, ms := 0, 0
	tier := aRow.Tier
	if used {
		toks = fRow.InputTokens + rRow.InputTokens
		ms = fRow.LatencyMS + rRow.LatencyMS
		tier = fmt.Sprintf("T0/T1+%s-agree", backend)
	}
	sRow := evalRow{
		Case:                 c.ID,
		Band:                 c.Band,
		Tier:                 tier,
		Type:                 sType,
		Link:                 sLink,
		Mention:              aRow.Mention,
		SupportMention:       suppContra,
		SupportStrict:        suppContra,
		SupportContradiction: suppContra,
		InputTokens:          toks,
		LatencyMS:            ms,
	}
	mRow := sRow
	mRow.Type = mType
	mRow.Link = mLink
	return sRow, mRow, used
}

func agreeStrict(fwdChoice, revChoice string) string {
	if fwdChoice != "unsure" && fwdChoice == revChoice {
		return fwdChoice
	}
	return "unsure"
}

func agreeMean(fwdRaw, revRaw string, fwdConf, revConf *float64, floor float64) string {
	if fwdRaw != "" && fwdRaw != "unsure" && fwdRaw == revRaw && fwdConf != nil && revConf != nil {
		if (*fwdConf+*revConf)/2.0 >= floor {
			return fwdRaw
		}
	}
	return "unsure"
}

func verifyEmojiSpanFixture(fixturePath string) (bool, bool, bool, error) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		return false, false, false, err
	}
	var entry cachedEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return false, false, false, err
	}
	state := entry.Request.State
	runes := []rune(state)
	u16 := utf16.Encode(runes)
	spans := entry.Response.Answers.Mentions.Spans["software"]
	if len(spans) == 0 {
		return false, false, false, fmt.Errorf("no software spans in %s", fixturePath)
	}
	sp := spans[0]
	cpOK := sp.Start >= 0 && sp.End <= len(runes) && string(runes[sp.Start:sp.End]) == sp.Text
	u16OK := sp.Start >= 0 && sp.End <= len(u16) && string(utf16.Decode(u16[sp.Start:sp.End])) == sp.Text
	b8OK := sp.Start >= 0 && sp.End <= len(state) && state[sp.Start:sp.End] == sp.Text
	return cpOK, u16OK, b8OK, nil
}
