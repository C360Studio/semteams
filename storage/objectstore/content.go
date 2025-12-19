package objectstore

import (
	"encoding/json"
	"time"
)

// BinaryRef is a reference to binary content stored separately in ObjectStore.
// Binary data is stored as raw bytes (no base64) under its own key,
// and the JSON metadata envelope references it via this struct.
type BinaryRef struct {
	// ContentType is the MIME type of the binary content.
	// Examples: "image/jpeg", "video/mp4", "application/pdf"
	ContentType string `json:"content_type"`

	// Size is the size of the binary content in bytes.
	Size int64 `json:"size"`

	// Key is the ObjectStore key where the binary data is stored.
	Key string `json:"key"`
}

// StoredContent is the envelope stored in ObjectStore for ContentStorable payloads.
// It contains the raw content fields along with the semantic mapping that describes
// how to interpret the content structure.
//
// This type is stored in ObjectStore and retrieved by consumers (like embedding workers)
// who need access to the raw content. The ContentFields map provides a semantic role
// mapping that tells consumers how to find specific content types (body, abstract, title)
// without hardcoding field names.
//
// For BinaryStorable payloads, binary content is stored separately (as raw bytes)
// and referenced via BinaryRefs. This avoids base64 bloat in the JSON envelope.
//
// Storage flow:
//  1. Processor receives raw document
//  2. Processor creates ContentStorable with RawContent() and ContentFields()
//  3. ObjectStore.StoreContent() serializes this StoredContent envelope
//  4. Consumers fetch via StorageRef and use ContentFields to find text
//
// Example stored JSON (with binary refs):
//
//	{
//	    "entity_id": "acme.logistics.content.video.training.vid-001",
//	    "fields": {
//	        "title": "Safety Training Video",
//	        "description": "Comprehensive safety training"
//	    },
//	    "binary_refs": {
//	        "video": {
//	            "content_type": "video/mp4",
//	            "size": 5242880,
//	            "key": "binary/2025/01/15/vid-001_video_1234567890"
//	        }
//	    },
//	    "content_fields": {
//	        "title": "title",
//	        "abstract": "description",
//	        "media": "video"
//	    },
//	    "stored_at": "2025-01-15T10:30:00Z"
//	}
type StoredContent struct {
	// EntityID is the federated entity ID this content belongs to
	EntityID string `json:"entity_id"`

	// Fields contains the raw text content by field name.
	// Field names here match the values in ContentFields.
	// Example: {"title": "Safety Manual", "body": "Full text...", "description": "Brief summary"}
	Fields map[string]string `json:"fields,omitempty"`

	// BinaryRefs contains references to binary content stored separately.
	// The actual binary data is stored under BinaryRef.Key.
	// Example: {"video": {ContentType: "video/mp4", Size: 5242880, Key: "binary/..."}}
	BinaryRefs map[string]BinaryRef `json:"binary_refs,omitempty"`

	// ContentFields maps semantic roles to field names in Fields or BinaryRefs.
	// This enables consumers to find content without hardcoding field names.
	// Standard roles: "body", "abstract", "title", "media", "thumbnail"
	// Example: {"body": "body", "abstract": "description", "title": "title", "media": "video"}
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

// NewStoredContentWithBinary creates a StoredContent envelope with both text and binary content.
func NewStoredContentWithBinary(entityID string, fields, contentFields map[string]string, binaryRefs map[string]BinaryRef) *StoredContent {
	return &StoredContent{
		EntityID:      entityID,
		Fields:        fields,
		BinaryRefs:    binaryRefs,
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

// HasRole returns true if the semantic role is mapped to a non-empty field or binary ref.
func (s *StoredContent) HasRole(role string) bool {
	fieldName, ok := s.ContentFields[role]
	if !ok {
		return false
	}
	// Check text fields first
	if content, ok := s.Fields[fieldName]; ok && content != "" {
		return true
	}
	// Check binary refs
	if _, ok := s.BinaryRefs[fieldName]; ok {
		return true
	}
	return false
}

// GetBinaryRefByRole returns the binary reference for a semantic role.
// Returns nil if the role is not mapped to a binary field.
func (s *StoredContent) GetBinaryRefByRole(role string) *BinaryRef {
	fieldName, ok := s.ContentFields[role]
	if !ok {
		return nil
	}
	if ref, ok := s.BinaryRefs[fieldName]; ok {
		return &ref
	}
	return nil
}

// HasBinaryContent returns true if this StoredContent has any binary references.
func (s *StoredContent) HasBinaryContent() bool {
	return len(s.BinaryRefs) > 0
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
