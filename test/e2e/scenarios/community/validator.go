package community

import (
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/graph/clustering"
)

// Validator validates communities against ground truth expectations.
type Validator struct {
	expectations []Expectation
}

// NewValidator creates a validator with the given expectations.
func NewValidator(expectations []Expectation) *Validator {
	return &Validator{expectations: expectations}
}

// NewDefaultValidator creates a validator with default expectations.
func NewDefaultValidator() *Validator {
	return NewValidator(DefaultExpectations())
}

// Validate checks communities against all expectations and returns results.
func (v *Validator) Validate(communities []*clustering.Community) *ValidationResult {
	result := &ValidationResult{
		ExpectationsTotal: len(v.expectations),
	}

	// Build entity -> community mapping for efficient lookup
	entityToCommunity := make(map[string]*clustering.Community)
	for _, comm := range communities {
		for _, member := range comm.Members {
			entityToCommunity[member] = comm
		}
	}

	for _, exp := range v.expectations {
		violations := v.validateExpectation(exp, entityToCommunity)
		if len(violations) == 0 {
			result.ExpectationsPassed++
		} else {
			result.Violations = append(result.Violations, violations...)
		}
	}

	return result
}

// validateExpectation checks a single expectation against the community mapping.
func (v *Validator) validateExpectation(exp Expectation, entityToCommunity map[string]*clustering.Community) []Violation {
	var violations []Violation

	// Find entities matching MustContain patterns
	mustContainMatches := make(map[string]string) // pattern -> matched entity ID
	for _, pattern := range exp.MustContain {
		matchedEntity, matchedComm := v.findEntityByPattern(pattern, entityToCommunity)
		if matchedEntity == "" {
			violations = append(violations, Violation{
				ExpectationName: exp.Name,
				Type:            ViolationEntityNotFound,
				Details:         fmt.Sprintf("entity matching pattern %q not found in any community", pattern),
				Entities:        []string{pattern},
			})
		} else {
			mustContainMatches[pattern] = matchedEntity
			// Store community for later checks
			if matchedComm != nil {
				mustContainMatches[pattern+"_comm"] = matchedComm.ID
			}
		}
	}

	// Check that all MustContain entities are in the same community
	if len(exp.MustContain) > 1 && len(violations) == 0 {
		var firstCommunityID string
		var entitiesInWrongCommunity []string
		var communityIDs []string
		communitySet := make(map[string]bool)

		for _, pattern := range exp.MustContain {
			commID := mustContainMatches[pattern+"_comm"]
			communitySet[commID] = true
			if firstCommunityID == "" {
				firstCommunityID = commID
			} else if commID != firstCommunityID {
				entitiesInWrongCommunity = append(entitiesInWrongCommunity, mustContainMatches[pattern])
			}
		}

		for id := range communitySet {
			communityIDs = append(communityIDs, id)
		}

		if len(communitySet) > 1 {
			violations = append(violations, Violation{
				ExpectationName: exp.Name,
				Type:            ViolationNotGrouped,
				Details: fmt.Sprintf("entities expected in same community are split across %d communities",
					len(communitySet)),
				Entities:     append([]string{mustContainMatches[exp.MustContain[0]]}, entitiesInWrongCommunity...),
				CommunityIDs: communityIDs,
			})
		}
	}

	// Check MustNotContain - these should NOT be in the same community as MustContain
	if len(exp.MustNotContain) > 0 && len(violations) == 0 && len(exp.MustContain) > 0 {
		// Get the community ID of the first MustContain entity
		mustContainCommID := mustContainMatches[exp.MustContain[0]+"_comm"]

		for _, excludePattern := range exp.MustNotContain {
			matchedEntity, matchedComm := v.findEntityByPattern(excludePattern, entityToCommunity)
			if matchedEntity != "" && matchedComm != nil && matchedComm.ID == mustContainCommID {
				violations = append(violations, Violation{
					ExpectationName: exp.Name,
					Type:            ViolationUnexpectedGrouped,
					Details: fmt.Sprintf("entity %q should not be in same community as %q",
						matchedEntity, mustContainMatches[exp.MustContain[0]]),
					Entities:     []string{matchedEntity, mustContainMatches[exp.MustContain[0]]},
					CommunityIDs: []string{matchedComm.ID},
				})
			}
		}
	}

	// Check MinSize if specified
	if exp.MinSize > 0 && len(violations) == 0 && len(exp.MustContain) > 0 {
		commID := mustContainMatches[exp.MustContain[0]+"_comm"]
		for _, comm := range entityToCommunity {
			if comm.ID == commID && len(comm.Members) < exp.MinSize {
				violations = append(violations, Violation{
					ExpectationName: exp.Name,
					Type:            ViolationCommunityTooSmall,
					Details: fmt.Sprintf("community %s has %d members, expected at least %d",
						comm.ID, len(comm.Members), exp.MinSize),
					CommunityIDs: []string{comm.ID},
				})
				break
			}
		}
	}

	return violations
}

// findEntityByPattern finds the first entity ID matching the pattern in any community.
func (v *Validator) findEntityByPattern(pattern string, entityToCommunity map[string]*clustering.Community) (string, *clustering.Community) {
	patternLower := strings.ToLower(pattern)
	for entityID, comm := range entityToCommunity {
		if strings.Contains(strings.ToLower(entityID), patternLower) {
			return entityID, comm
		}
	}
	return "", nil
}
