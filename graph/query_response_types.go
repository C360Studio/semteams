// Package graph provides query response type aliases for common query patterns.
package graph

// --- Type Aliases for Common Response Types ---

// OutgoingQueryResponse is the response type for outgoing relationship queries.
type OutgoingQueryResponse = QueryResponse[OutgoingRelationshipsData]

// IncomingQueryResponse is the response type for incoming relationship queries.
type IncomingQueryResponse = QueryResponse[IncomingRelationshipsData]

// AliasQueryResponse is the response type for alias resolution queries.
type AliasQueryResponse = QueryResponse[AliasData]

// PredicateQueryResponse is the response type for predicate queries.
type PredicateQueryResponse = QueryResponse[PredicateData]

// PredicateListQueryResponse is the response type for predicate list queries.
type PredicateListQueryResponse = QueryResponse[PredicateListData]

// PredicateStatsQueryResponse is the response type for predicate stats queries.
type PredicateStatsQueryResponse = QueryResponse[PredicateStatsData]

// CompoundPredicateQueryResponse is the response type for compound predicate queries.
type CompoundPredicateQueryResponse = QueryResponse[CompoundPredicateData]
