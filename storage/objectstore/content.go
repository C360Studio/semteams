package objectstore

import (
	"encoding/json"
	"time"
)

// StoredContent is the envelope stored in ObjectStore for ContentStorable payloads.
// It contains the raw content fields along with the semantic mapping that describes
// how to interpret the content structure.
//
// This type is stored in ObjectStore and retrieved by consumers (like embedding workers)
// who need access to the raw content. The ContentFields map provides a semantic role
// mapping that tells consumers how to find specific content types (body, abstract, title)
// without hardcoding field names.
//
// Storage flow:
//  1. Processor receives raw document
//  2. Processor creates ContentStorable with RawContent() and ContentFields()
//  3. ObjectStore.StoreContent() serializes this StoredContent envelope
//  4. Consumers fetch via StorageRef and use ContentFields to find text
//
// Example stored JSON:
//
//	{
//	    "entity_id": "acme.logistics.content.document.safety.doc-001",
//	    "fields": {
//	        "title": "Safety Manual",
//	        "description": "Comprehensive safety guidelines",
//	        "body": "Full document text..."
//	    },
//	    "content_fields": {
//	        "title": "title",
//	        "abstract": "description",
//	        "body": "body"
//	    },
//	    "stored_at": "2025-01-15T10:30:00Z"
//	}
type StoredContent struct {
	// EntityID is the federated entity ID this content belongs to
	EntityID string `json:"entity_id"`

	// Fields contains the raw content by field name.
	// Field names here match the values in ContentFields.
	// Example: {"title": "Safety Manual", "body": "Full text...", "description": "Brief summary"}
	Fields map[string]string `json:"fields"`

	// ContentFields maps semantic roles to field names in Fields.
	// This enables consumers to find content without hardcoding field names.
	// Standard roles: "body", "abstract", "title"
	// Example: {"body": "body", "abstract": "description", "title": "title"}
	ContentFields map[string]string `json:"content_fields"`

	// StoredAt is when the content was stored
	StoredAt time.Time `json:"stored_at"`
}

// NewStoredContent creates a StoredContent envelope from entity ID, raw content, and field mapping.
func NewStoredContent(entityID string, fields, contentFields map[string]string) *StoredContent {
	return &StoredContent{
		EntityID:      entityID,
		Fields:        fields,
		ContentFields: contentFields,
		StoredAt:      time.Now(),
	}
}

// GetFieldByRole returns the content for a semantic role (e.g., "body", "abstract").
// Returns empty string if the role is not mapped or the field is empty.
func (s *StoredContent) GetFieldByRole(role string) string {
	fieldName, ok := s.ContentFields[role]
	if !ok {
		return ""
	}
	return s.Fields[fieldName]
}

// HasRole returns true if the semantic role is mapped to a non-empty field.
func (s *StoredContent) HasRole(role string) bool {
	fieldName, ok := s.ContentFields[role]
	if !ok {
		return false
	}
	content, ok := s.Fields[fieldName]
	return ok && content != ""
}

// MarshalJSON provides JSON serialization for StoredContent.
func (s *StoredContent) MarshalJSON() ([]byte, error) {
	type Alias StoredContent
	return json.Marshal((*Alias)(s))
}

// UnmarshalJSON provides JSON deserialization for StoredContent.
func (s *StoredContent) UnmarshalJSON(data []byte) error {
	type Alias StoredContent
	return json.Unmarshal(data, (*Alias)(s))
}
