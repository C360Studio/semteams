package entityextraction

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	repoRE      = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	specRE      = regexp.MustCompile(`^(?i:RFC\s*\d+|ADR-\d+|CVE-\d{4}-\d+)$`)
	wsCollapse  = regexp.MustCompile(`\s+`)
	nonAlnumSl  = regexp.MustCompile(`[^a-z0-9/]+`)
	nonAlnumTok = regexp.MustCompile(`[^a-z0-9]+`)
)

var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"by": true, "for": true, "from": true, "in": true, "into": true, "is": true,
	"it": true, "of": true, "on": true, "or": true, "over": true, "that": true,
	"the": true, "to": true, "under": true, "with": true, "v1": true, "v2": true,
	"v5": true, "0": true, "1": true, "2": true, "5": true, "protocol": true,
	"specification": true, "broker": true, "server": true, "collector": true,
	"test": true, "runner": true, "cluster": true, "router": true, "gateway": true,
}

var precomposedMap = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
	'À': 'a', 'Á': 'a', 'Â': 'a', 'Ã': 'a', 'Ä': 'a', 'Å': 'a',
	'ç': 'c', 'Ç': 'c',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'È': 'e', 'É': 'e', 'Ê': 'e', 'Ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'Ì': 'i', 'Í': 'i', 'Î': 'i', 'Ï': 'i',
	'ñ': 'n', 'Ñ': 'n',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o',
	'Ò': 'o', 'Ó': 'o', 'Ô': 'o', 'Õ': 'o', 'Ö': 'o', 'Ø': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'Ù': 'u', 'Ú': 'u', 'Û': 'u', 'Ü': 'u',
	'ý': 'y', 'ÿ': 'y', 'Ý': 'y',
}

func foldText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if mapped, ok := precomposedMap[r]; ok {
			b.WriteRune(mapped)
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.TrimSpace(wsCollapse.ReplaceAllString(b.String(), " "))
}

func foldAlnum(s string) string {
	f := foldText(s)
	return strings.TrimSpace(nonAlnumSl.ReplaceAllString(f, " "))
}

func foldCompact(s string) string {
	f := foldText(s)
	return nonAlnumSl.ReplaceAllString(f, "")
}

func tokenize(s string) []string {
	parts := nonAlnumTok.Split(foldText(s), -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" && !stopwords[p] {
			out = append(out, p)
		}
	}
	return out
}

func tokenSet(s string) map[string]bool {
	toks := tokenize(s)
	out := make(map[string]bool, len(toks))
	for _, t := range toks {
		out[t] = true
	}
	return out
}

type scoredCandidate struct {
	Entity catalogEntity
	Score  float64
}

type algoFloor struct {
	catalog      []catalogEntity
	exactIndex   map[string][]catalogEntity
	compactIndex map[string][]catalogEntity
	nameTokens   map[string]map[string]bool
	descTokens   map[string]map[string]bool
	idf          map[string]float64
}

func newAlgoFloor(seedCatalog []catalogEntity) *algoFloor {
	af := &algoFloor{
		catalog:      seedCatalog,
		exactIndex:   make(map[string][]catalogEntity),
		compactIndex: make(map[string][]catalogEntity),
		nameTokens:   make(map[string]map[string]bool),
		descTokens:   make(map[string]map[string]bool),
		idf:          make(map[string]float64),
	}
	df := make(map[string]int)
	for _, ent := range seedCatalog {
		af.indexEntitySurfaces(ent)
		nSurf := strings.Join(append([]string{ent.Name}, ent.Aliases...), " ")
		nToks := tokenSet(nSurf)
		dToks := tokenSet(ent.Description)
		af.nameTokens[ent.ID] = nToks
		af.descTokens[ent.ID] = dToks
		seen := make(map[string]bool)
		for t := range nToks {
			seen[t] = true
		}
		for t := range dToks {
			seen[t] = true
		}
		for t := range seen {
			df[t]++
		}
	}
	nDocs := math.Max(float64(len(seedCatalog)), 1.0)
	for t, c := range df {
		cf := float64(c)
		af.idf[t] = math.Log(1.0 + (nDocs-cf+0.5)/(cf+0.5))
	}
	return af
}

func (af *algoFloor) indexEntitySurfaces(ent catalogEntity) {
	surfaces := append([]string{ent.Name}, ent.Aliases...)
	for _, sf := range surfaces {
		for _, key := range []string{foldText(sf), foldAlnum(sf)} {
			if key != "" && !containsEntity(af.exactIndex[key], ent.ID) {
				af.exactIndex[key] = append(af.exactIndex[key], ent)
			}
		}
		ckey := foldCompact(sf)
		if ckey != "" && !containsEntity(af.compactIndex[ckey], ent.ID) {
			af.compactIndex[ckey] = append(af.compactIndex[ckey], ent)
		}
	}
}

func containsEntity(list []catalogEntity, id string) bool {
	for _, e := range list {
		if e.ID == id {
			return true
		}
	}
	return false
}

// portfolioOnlyCatalog returns the day-one production catalog subset (only
// C360 / SemTeams / SemEngine / SemStreams / semvocab / semsparql / NATS / ADR
// entries, excluding external organizations, people, benchmarks, and third-party tools).
func portfolioOnlyCatalog(seedCatalog []catalogEntity) []catalogEntity {
	out := make([]catalogEntity, 0, 8)
	for _, ent := range seedCatalog {
		if strings.Contains(ent.ID, "c360studio") ||
			strings.Contains(ent.ID, ":sem") ||
			strings.Contains(ent.ID, "nats-jetstream") ||
			strings.Contains(ent.ID, "adr-061") {
			out = append(out, ent)
		}
	}
	return out
}

func (af *algoFloor) t1Candidates(name string, topK int) []scoredCandidate {
	qToks := tokenSet(name)
	qCompact := foldCompact(name)
	var scored []scoredCandidate
	for _, ent := range af.catalog {
		score := af.scoreCandidate(ent, qToks, qCompact)
		if score > 0.0 {
			scored = append(scored, scoredCandidate{
				Entity: ent,
				Score:  math.Round(score*10000) / 10000,
			})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Entity.ID < scored[j].Entity.ID
	})
	if len(scored) > topK {
		return scored[:topK]
	}
	return scored
}

func (af *algoFloor) scoreCandidate(ent catalogEntity, qToks map[string]bool, qCompact string) float64 {
	nToks := af.nameTokens[ent.ID]
	dToks := af.descTokens[ent.ID]
	var nOverlapSum, dOverlapSum, denSum float64
	hasNOverlap := false
	for t := range qToks {
		w, ok := af.idf[t]
		if !ok {
			w = 1.0
		}
		denSum += w
		if nToks[t] {
			hasNOverlap = true
			nOverlapSum += w
		} else if dToks[t] {
			dOverlapSum += w
		}
	}
	score := 0.0
	if hasNOverlap && len(qToks) > 0 {
		score = (nOverlapSum + 0.5*dOverlapSum) / math.Max(denSum, 1e-6)
	}
	surfaces := append([]string{ent.Name}, ent.Aliases...)
	for _, sf := range surfaces {
		sfc := foldCompact(sf)
		if sfc != "" && sfc == qCompact && score < 1.0 {
			score = 1.0
		}
	}
	return score
}

func (af *algoFloor) checkMention(name string, linked *catalogEntity, quote string) bool {
	qFold := foldAlnum(quote)
	qCompact := foldCompact(quote)
	surfaces := []string{name}
	if linked != nil {
		surfaces = append(surfaces, linked.Name)
		surfaces = append(surfaces, linked.Aliases...)
	}
	for _, sf := range surfaces {
		sfAlnum := foldAlnum(sf)
		sfComp := foldCompact(sf)
		if sfAlnum != "" && strings.Contains(qFold, sfAlnum) {
			return true
		}
		if len(sfComp) >= 4 && strings.Contains(qCompact, sfComp) {
			return true
		}
	}
	qToks := tokenSet(quote)
	var nameToks []string
	for _, t := range tokenize(name) {
		if len(t) >= 3 {
			nameToks = append(nameToks, t)
		}
	}
	if len(nameToks) == 0 {
		return false
	}
	for _, t := range nameToks {
		if !qToks[t] {
			return false
		}
	}
	return true
}

func (af *algoFloor) hasContradictionCue(role, quote string) bool {
	qLow := foldText(quote)
	rLow := foldText(role)
	negCues := []string{"never", "no ", "not ", "neither", "without", "read-only", "only for"}
	qHas := false
	for _, cue := range negCues {
		if strings.Contains(qLow, cue) {
			qHas = true
		}
		if strings.Contains(rLow, cue) {
			return false
		}
	}
	return qHas
}

func (af *algoFloor) exactHitsFor(name string) []catalogEntity {
	var hits []catalogEntity
	for _, ent := range af.exactIndex[foldText(name)] {
		if !containsEntity(hits, ent.ID) {
			hits = append(hits, ent)
		}
	}
	for _, ent := range af.exactIndex[foldAlnum(name)] {
		if !containsEntity(hits, ent.ID) {
			hits = append(hits, ent)
		}
	}
	for _, ent := range af.compactIndex[foldCompact(name)] {
		if !containsEntity(hits, ent.ID) {
			hits = append(hits, ent)
		}
	}
	return hits
}

func (af *algoFloor) evaluateCase(c corpusCase) evalRow {
	exactHits := af.exactHitsFor(c.Name)
	candidates := af.t1Candidates(c.Name, 5)
	predType, predLink, tier, linked := af.resolveTypeAndLink(c.Name, exactHits, candidates)
	mention := af.checkMention(c.Name, linked, c.Quote)

	var suppMention, suppStrict, suppContra any
	if !mention {
		suppMention = false
		suppStrict = false
		suppContra = false
	} else {
		suppMention = true
		suppStrict = "unsure"
		suppContra = true
	}
	candIDs := make([]string, len(candidates))
	for i, cand := range candidates {
		candIDs[i] = cand.Entity.ID
	}
	one := 1.0
	var tConf, lConf *float64
	if predType != "unsure" {
		tConf = &one
	}
	if predLink != "unsure" {
		lConf = &one
	}
	mCopy := mention
	return evalRow{
		Case:                 c.ID,
		Band:                 c.Band,
		Tier:                 tier,
		Type:                 predType,
		TypeConf:             tConf,
		Link:                 predLink,
		LinkConf:             lConf,
		Mention:              &mCopy,
		SupportMention:       suppMention,
		SupportStrict:        suppStrict,
		SupportContradiction: suppContra,
		T1Candidates:         candIDs,
	}
}

func (af *algoFloor) resolveTypeAndLink(
	name string,
	exactHits []catalogEntity,
	candidates []scoredCandidate,
) (string, string, string, *catalogEntity) {
	if len(exactHits) == 1 {
		hit := exactHits[0]
		return hit.Type, hit.ID, "T0", &hit
	}
	if len(exactHits) > 1 {
		return "unsure", "unsure", "T0-ambiguous", nil
	}
	normN := foldText(name)
	if repoRE.MatchString(normN) {
		return "repository", "new", "T0-pattern", nil
	}
	if specRE.MatchString(normN) {
		return "specification", "new", "T0-pattern", nil
	}
	if len(candidates) == 1 && candidates[0].Score >= 0.70 {
		hit := candidates[0].Entity
		return hit.Type, hit.ID, "T1-bm25", &hit
	}
	if len(candidates) >= 2 && candidates[0].Score >= 0.70 && (candidates[0].Score-candidates[1].Score) >= 0.25 {
		hit := candidates[0].Entity
		return hit.Type, hit.ID, "T1-bm25", &hit
	}
	if len(candidates) == 0 {
		return "unsure", "new", "T0-novel", nil
	}
	return "unsure", "unsure", "T0", nil
}
