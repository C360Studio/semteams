package message

// ContentStorable extends Storable for payloads with large content fields.
//
// This interface enables the "process → store → graph" pattern where:
//  1. Processor creates semantic understanding (triples with metadata only)
//  2. ObjectStore stores raw content and returns StorageReference
//  3. GraphProcessor receives ContentStorable with both semantics and content reference
//  4. EmbeddingWorker uses ContentFields to find text for embedding
//
// The key insight is that ContentStorable separates metadata (in triples) from
// content (in ObjectStore), while providing a semantic map of content structure
// via ContentFields. This avoids bloating triples with large text and enables
// efficient embedding extraction without hardcoded field name coupling.
//
// Example implementation:
//
//	type Document struct {
//	    Title       string
//	    Description string
//	    Body        string
//	    storageRef  *StorageReference
//	}
//
//	func (d *Document) EntityID() string { return d.entityID }
//	func (d *Document) Triples() []Triple { return metadataTriples } // NO body
//	func (d *Document) StorageRef() *StorageReference { return d.storageRef }
//
//	func (d *Document) ContentFields() map[string]string {
//	    return map[string]string{
//	        "body":     "body",        // semantic role → field name
//	        "abstract": "description",
//	        "title":    "title",
//	    }
//	}
//
//	func (d *Document) RawContent() map[string]string {
//	    return map[string]string{
//	        "title":       d.Title,
//	        "description": d.Description,
//	        "body":        d.Body,
//	    }
//	}
type ContentStorable interface {
	Storable // EntityID() + Triples() + StorageRef()

	// ContentFields returns semantic role → field name mapping.
	//
	// This tells consumers how to find content in the stored data without
	// hardcoding field names. Keys are semantic roles understood by consumers
	// (like embedding workers), values are field names in RawContent().
	//
	// Standard semantic roles:
	//   - "body":     Primary text content (full document text)
	//   - "abstract": Brief summary or description
	//   - "title":    Document title
	//
	// Example return value:
	//   {"body": "content", "abstract": "description", "title": "title"}
	//
	// The map should only include roles that have non-empty content.
	ContentFields() map[string]string

	// RawContent returns the content to store in ObjectStore.
	//
	// Field names in this map should match values in ContentFields().
	// This is what gets serialized and stored; consumers retrieve it
	// via StorageRef and use ContentFields to find specific content.
	//
	// Example return value:
	//   {"title": "Safety Manual", "content": "Full document text...", "description": "Brief summary"}
	RawContent() map[string]string
}

// ContentRole constants define standard semantic roles for ContentFields.
// Using these constants ensures consistency across implementations.
const (
	// ContentRoleBody is the primary text content (full document text).
	// Embedding workers prioritize this role for text extraction.
	ContentRoleBody = "body"

	// ContentRoleAbstract is a brief summary or description.
	// Used when body is not available or for additional context.
	ContentRoleAbstract = "abstract"

	// ContentRoleTitle is the document or entity title.
	// Typically included in embeddings for context.
	ContentRoleTitle = "title"
)
