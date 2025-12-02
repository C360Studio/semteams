package graph

// Bucket name constants for NATS KV storage
const (
	// Primary entity storage
	BucketEntityStates = "ENTITY_STATES"

	// Graph relationship indexes
	BucketPredicateIndex = "PREDICATE_INDEX"
	BucketIncomingIndex  = "INCOMING_INDEX"
	BucketOutgoingIndex  = "OUTGOING_INDEX"

	// Lookup indexes
	BucketAliasIndex    = "ALIAS_INDEX"
	BucketSpatialIndex  = "SPATIAL_INDEX"
	BucketTemporalIndex = "TEMPORAL_INDEX"
)
