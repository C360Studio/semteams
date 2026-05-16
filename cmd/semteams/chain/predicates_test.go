package chain

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

// TestPredicates_RegisteredInVocabulary pins the registration contract:
// every exported chain predicate constant must show up in the upstream
// vocabulary registry with non-empty metadata. A new predicate added to
// predicates.go without a matching vocabulary.Register() call would
// silently ship as an "unknown" predicate to RDF export and ops-agent
// consumers; this test catches the omission at build time.
func TestPredicates_RegisteredInVocabulary(t *testing.T) {
	cases := []struct {
		constName string
		predicate string
	}{
		{"PredicateSlugStem", PredicateSlugStem},
		{"PredicateResearchArtifactLoop", PredicateResearchArtifactLoop},
		{"PredicatePlanLoop", PredicatePlanLoop},
		{"PredicatePlanReviewerLoop", PredicatePlanReviewerLoop},
		{"PredicateChainMode", PredicateChainMode},
	}
	for _, tc := range cases {
		meta := vocabulary.GetPredicateMetadata(tc.predicate)
		if meta == nil {
			t.Errorf("%s (%q) not registered in vocabulary — add a vocabulary.Register() call in predicates.go init()", tc.constName, tc.predicate)
			continue
		}
		if meta.Description == "" {
			t.Errorf("%s (%q) registered without a description — supply WithDescription so RDF export and discoverability tools have human-readable text", tc.constName, tc.predicate)
		}
		if meta.DataType == "" {
			t.Errorf("%s (%q) registered without a data type — supply WithDataType so consumers know the on-the-wire shape", tc.constName, tc.predicate)
		}
		if meta.Domain != "chain" {
			t.Errorf("%s (%q) parsed domain = %q, want %q (chain.* prefix is required for chain-vocabulary discoverability)", tc.constName, tc.predicate, meta.Domain, "chain")
		}
	}
}
