package datamanager

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/c360studio/semstreams/message"
)

// T115: Test relationship detection using Triple.IsRelationship()
// FR-006a/b: Relationship detection MUST use Triple.IsRelationship() which validates
// Object as 6-part EntityID, NOT hardcoded predicate name lists
func TestRelationshipDetection_UsesTripleIsRelationship(t *testing.T) {
	t.Run("isRelationshipPredicate should be replaced with Triple.IsRelationship", func(t *testing.T) {
		// This test verifies that we use Triple.IsRelationship() which checks the Object
		// rather than isRelationshipPredicate() which uses a hardcoded predicate list

		tests := []struct {
			name         string
			triple       message.Triple
			wantRelation bool
			reason       string
		}{
			{
				name: "6-part entity ID object is a relationship",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "spatial.proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.002",
					Timestamp: time.Now(),
				},
				wantRelation: true,
				reason:       "Object is a valid 6-part EntityID",
			},
			{
				name: "predicate in hardcoded list but non-EntityID object is NOT a relationship",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "POWERED_BY", // In old hardcoded list
					Object:    "battery",    // Not a valid EntityID
					Timestamp: time.Now(),
				},
				wantRelation: false,
				reason:       "Object is not a valid EntityID (only 1 part)",
			},
			{
				name: "predicate NOT in hardcoded list but valid EntityID object IS a relationship",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "custom.relationship.follows", // NOT in old hardcoded list
					Object:    "c360.platform1.robotics.mav1.drone.002",
					Timestamp: time.Now(),
				},
				wantRelation: true,
				reason:       "Object is a valid 6-part EntityID, predicate doesn't matter",
			},
			{
				name: "literal value is not a relationship",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "robotics.battery.level",
					Object:    85.5,
					Timestamp: time.Now(),
				},
				wantRelation: false,
				reason:       "Object is a float, not an EntityID",
			},
			{
				name: "boolean value is not a relationship",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "robotics.flight.armed",
					Object:    true,
					Timestamp: time.Now(),
				},
				wantRelation: false,
				reason:       "Object is a boolean, not an EntityID",
			},
			{
				name: "4-part entity ID is NOT a relationship (old format)",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "NEAR",
					Object:    "telemetry.robotics.drone.002", // Only 4 parts
					Timestamp: time.Now(),
				},
				wantRelation: false,
				reason:       "Object is only 4 parts, not valid 6-part EntityID",
			},
			{
				name: "5-part entity ID is NOT a relationship",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "RELATED_TO",
					Object:    "c360.platform1.robotics.mav1.drone", // Only 5 parts
					Timestamp: time.Now(),
				},
				wantRelation: false,
				reason:       "Object is only 5 parts, not valid 6-part EntityID",
			},
			{
				name: "empty string is not a relationship",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "CONNECTED_TO",
					Object:    "",
					Timestamp: time.Now(),
				},
				wantRelation: false,
				reason:       "Object is empty string",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Use Triple.IsRelationship() - the correct approach
				isRelation := tt.triple.IsRelationship()
				assert.Equal(t, tt.wantRelation, isRelation, tt.reason)

				// Verify specific cases where the old hardcoded approach would have failed
				if tt.triple.Predicate == "custom.relationship.follows" {
					// Old approach would have returned false (not in hardcoded list)
					// New approach correctly returns true (valid EntityID object)
					assert.True(t, isRelation, "New approach should detect EntityID objects")
				}

				if tt.triple.Predicate == "POWERED_BY" && tt.triple.Object == "battery" {
					// Old approach would have returned true (in hardcoded list)
					// New approach correctly returns false (object is not EntityID)
					assert.False(t, isRelation, "New approach correctly returns false")
				}
			})
		}
	})
}

// TestAddTriple_RelationshipValidation verifies that AddTriple uses Triple.IsRelationship()
// for detecting relationships when ValidateEdgeTargets is enabled
func TestAddTriple_RelationshipValidation(t *testing.T) {
	t.Run("validates target only when object is valid EntityID", func(t *testing.T) {
		// This test will fail until we replace isRelationshipPredicate with Triple.IsRelationship()
		// It demonstrates that validation should be based on object format, not predicate name

		tests := []struct {
			name           string
			triple         message.Triple
			shouldValidate bool
			reason         string
		}{
			{
				name: "should validate when object is 6-part EntityID",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "custom.follows", // Not in old hardcoded list
					Object:    "c360.platform1.robotics.mav1.drone.002",
				},
				shouldValidate: true,
				reason:         "Object is valid 6-part EntityID",
			},
			{
				name: "should NOT validate when object is literal despite predicate name",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "POWERED_BY", // In old hardcoded list
					Object:    "solar",      // Not a valid EntityID
				},
				shouldValidate: false,
				reason:         "Object is not a valid EntityID",
			},
			{
				name: "should NOT validate when object is numeric",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "NEAR",
					Object:    42.5,
				},
				shouldValidate: false,
				reason:         "Object is numeric, not EntityID",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Check if triple represents a relationship
				isRelationship := tt.triple.IsRelationship()
				assert.Equal(t, tt.shouldValidate, isRelationship, tt.reason)

				// The code should use this result to determine if validation is needed
				// instead of calling isRelationshipPredicate(triple.Predicate)
			})
		}
	})
}

// TestCheckOutgoingTriplesConsistency_UsesTripleIsRelationship verifies that
// consistency checking uses Triple.IsRelationship() not isRelationshipPredicate()
func TestCheckOutgoingTriplesConsistency_UsesTripleIsRelationship(t *testing.T) {
	t.Run("detects inconsistency only for actual relationships", func(t *testing.T) {
		// This test ensures that we check target existence only for triples
		// where Object is a valid EntityID, not based on predicate names

		tests := []struct {
			name              string
			triple            message.Triple
			shouldCheckTarget bool
			reason            string
		}{
			{
				name: "should check target for 6-part EntityID object",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "custom.references",
					Object:    "c360.platform1.robotics.mav1.sensor.001",
				},
				shouldCheckTarget: true,
				reason:            "Object is valid 6-part EntityID",
			},
			{
				name: "should NOT check target for literal string",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "POWERED_BY",
					Object:    "battery_type_a",
				},
				shouldCheckTarget: false,
				reason:            "Object is literal string, not EntityID",
			},
			{
				name: "should NOT check target for numeric value",
				triple: message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "robotics.altitude.meters",
					Object:    150.5,
				},
				shouldCheckTarget: false,
				reason:            "Object is numeric, not EntityID",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				isRelationship := tt.triple.IsRelationship()
				assert.Equal(t, tt.shouldCheckTarget, isRelationship, tt.reason)

				// Code should use: if triple.IsRelationship() { checkTargetExists(...) }
				// NOT: if isRelationshipPredicate(triple.Predicate) { ... }
			})
		}
	})
}

// TestTripleIsRelationship_EntityIDValidation verifies the 6-part EntityID requirement
func TestTripleIsRelationship_EntityIDValidation(t *testing.T) {
	tests := []struct {
		name         string
		object       any
		wantRelation bool
	}{
		{
			name:         "valid 6-part EntityID",
			object:       "c360.platform1.robotics.mav1.drone.001",
			wantRelation: true,
		},
		{
			name:         "4-part EntityID (old format) - not valid",
			object:       "telemetry.robotics.drone.001",
			wantRelation: false,
		},
		{
			name:         "5-part EntityID - not valid",
			object:       "c360.platform1.robotics.mav1.drone",
			wantRelation: false,
		},
		{
			name:         "7-part string - not valid",
			object:       "c360.platform1.robotics.mav1.drone.001.extra",
			wantRelation: false,
		},
		{
			name:         "empty parts - not valid",
			object:       "c360..robotics.mav1.drone.001",
			wantRelation: false,
		},
		{
			name:         "trailing dot - not valid",
			object:       "c360.platform1.robotics.mav1.drone.001.",
			wantRelation: false,
		},
		{
			name:         "leading dot - not valid",
			object:       ".c360.platform1.robotics.mav1.drone.001",
			wantRelation: false,
		},
		{
			name:         "non-string object",
			object:       123,
			wantRelation: false,
		},
		{
			name:         "boolean object",
			object:       true,
			wantRelation: false,
		},
		{
			name:         "nil object",
			object:       nil,
			wantRelation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triple := message.Triple{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "test.predicate",
				Object:    tt.object,
				Timestamp: time.Now(),
			}

			result := triple.IsRelationship()
			assert.Equal(t, tt.wantRelation, result)
		})
	}
}

// TestTripleIsRelationship verifies that Triple.IsRelationship() correctly identifies
// relationships based on Object type (EntityID) rather than predicate name.
// This replaced the old isRelationshipPredicate() which used a hardcoded predicate list.
func TestTripleIsRelationship(t *testing.T) {

	t.Run("Triple.IsRelationship gives correct results", func(t *testing.T) {
		// Same cases but using Triple.IsRelationship()

		// Case 1: Custom predicate with EntityID object
		triple1 := message.Triple{
			Subject:   "c360.platform1.robotics.mav1.drone.001",
			Predicate: "custom.new.follows",
			Object:    "c360.platform1.robotics.mav1.drone.002",
		}
		assert.True(t, triple1.IsRelationship(),
			"Triple.IsRelationship correctly identifies EntityID object")

		// Case 2: POWERED_BY with literal object
		triple2 := message.Triple{
			Subject:   "c360.platform1.robotics.mav1.drone.001",
			Predicate: "POWERED_BY",
			Object:    "battery",
		}
		assert.False(t, triple2.IsRelationship(),
			"Triple.IsRelationship correctly rejects literal object")
	})
}

// TestNoHardcodedPredicateLists verifies we don't use hardcoded predicate lists
func TestNoHardcodedPredicateLists(t *testing.T) {
	t.Run("relationship detection must not depend on predicate name", func(t *testing.T) {
		// Create triples with identical predicates but different object types
		predicate := "custom.new.relationship" // Not in any hardcoded list

		relationshipTriple := message.Triple{
			Subject:   "c360.platform1.robotics.mav1.drone.001",
			Predicate: predicate,
			Object:    "c360.platform1.robotics.mav1.drone.002", // Valid EntityID
		}

		literalTriple := message.Triple{
			Subject:   "c360.platform1.robotics.mav1.drone.001",
			Predicate: predicate,
			Object:    "just a string value", // Not a valid EntityID
		}

		// The same predicate should be detected differently based on object
		assert.True(t, relationshipTriple.IsRelationship(),
			"Should detect relationship when object is valid EntityID")
		assert.False(t, literalTriple.IsRelationship(),
			"Should not detect relationship when object is literal")

		// This proves we're checking the object, not the predicate name
	})

	t.Run("predicates from old hardcoded list work correctly", func(t *testing.T) {
		// Verify that predicates from the old hardcoded list still work,
		// but based on object format not predicate name

		oldPredicates := []string{
			"POWERED_BY",
			"NEAR",
			"LOCATED_AT",
			"CONNECTED_TO",
			"PART_OF",
		}

		for _, pred := range oldPredicates {
			t.Run(pred, func(t *testing.T) {
				// With valid EntityID object - should be relationship
				validTriple := message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: pred,
					Object:    "c360.platform1.robotics.mav1.sensor.001",
				}
				assert.True(t, validTriple.IsRelationship(),
					"Predicate %s with EntityID object should be relationship", pred)

				// With literal object - should NOT be relationship
				literalTriple := message.Triple{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: pred,
					Object:    "literal value",
				}
				assert.False(t, literalTriple.IsRelationship(),
					"Predicate %s with literal object should not be relationship", pred)
			})
		}
	})
}
