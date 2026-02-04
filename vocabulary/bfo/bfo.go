package bfo

// BFO 2.0 (ISO 21838-2) IRI constants
//
// Basic Formal Ontology provides domain-neutral categories for ontology development.
// These IRIs use the OBO Foundry PURL format for stable, resolvable identifiers.
//
// Reference: https://basic-formal-ontology.org/

// Namespace is the base IRI prefix for all BFO terms.
const Namespace = "http://purl.obolibrary.org/obo/"

// Top-Level Categories
//
// BFO's root distinction: Entity splits into Continuant (persists) and Occurrent (unfolds).
const (
	// Entity is the root of all BFO entities.
	// Everything that exists is an entity.
	Entity = Namespace + "BFO_0000001"

	// Continuant represents entities that persist through time while maintaining identity.
	// Examples: objects, qualities, roles, spatial regions
	Continuant = Namespace + "BFO_0000002"

	// Occurrent represents entities that unfold or happen in time.
	// Examples: processes, events, temporal regions
	Occurrent = Namespace + "BFO_0000003"
)

// Continuant Subtypes
//
// Continuants divide into independent, specifically dependent, and generically dependent.
const (
	// IndependentContinuant is an entity that can exist on its own.
	// Includes material entities, spatial regions, and object boundaries.
	IndependentContinuant = Namespace + "BFO_0000004"

	// SpecificallyDependentContinuant depends on exactly one independent continuant.
	// Includes qualities, realizable entities (roles, dispositions, functions).
	SpecificallyDependentContinuant = Namespace + "BFO_0000020"

	// GenericallyDependentContinuant can migrate between bearers.
	// Includes information content entities (patterns that can be copied).
	GenericallyDependentContinuant = Namespace + "BFO_0000031"
)

// Independent Continuant Subtypes
//
// Material entities, objects, and spatial regions.
const (
	// MaterialEntity is an independent continuant with a material basis.
	// Has mass, occupies space, and is made of matter.
	MaterialEntity = Namespace + "BFO_0000040"

	// Object is a material entity that is spatially extended in 3D,
	// causally unified, and maximally self-connected.
	// Examples: a drone, a person, a building
	Object = Namespace + "BFO_0000030"

	// ObjectAggregate is a material entity consisting of multiple objects.
	// Examples: a fleet of drones, a flock of birds
	ObjectAggregate = Namespace + "BFO_0000027"

	// FiatObjectPart is a material entity that is part of an object
	// but whose boundaries are not physically demarcated.
	// Examples: the upper half of a drone, the northern region of a field
	FiatObjectPart = Namespace + "BFO_0000024"

	// Site is an immaterial entity consisting of a space bounded by material entities.
	// Examples: a hangar interior, a no-fly zone, a workspace
	Site = Namespace + "BFO_0000029"

	// SpatialRegion is a continuant entity that is part of space.
	SpatialRegion = Namespace + "BFO_0000006"

	// OneDimensionalSpatialRegion is a line or curve in space.
	OneDimensionalSpatialRegion = Namespace + "BFO_0000026"

	// TwoDimensionalSpatialRegion is a surface or area in space.
	TwoDimensionalSpatialRegion = Namespace + "BFO_0000009"

	// ThreeDimensionalSpatialRegion is a volume in space.
	ThreeDimensionalSpatialRegion = Namespace + "BFO_0000028"
)

// Specifically Dependent Continuant Subtypes
//
// Qualities and realizable entities (roles, dispositions, functions).
const (
	// Quality is an entity that is exhibited by its bearer.
	// Examples: color, temperature, mass, shape
	Quality = Namespace + "BFO_0000019"

	// RealizableEntity is a specifically dependent continuant
	// whose instances can be realized in processes.
	RealizableEntity = Namespace + "BFO_0000017"

	// Role is a realizable entity that exists because of externally imposed
	// expectations or specifications.
	// Examples: the role of pilot, the role of sensor
	Role = Namespace + "BFO_0000023"

	// Disposition is a realizable entity that exists because of
	// physical make-up of the bearer.
	// Examples: fragility, conductivity, magnetism
	Disposition = Namespace + "BFO_0000016"

	// Function is a disposition that exists because the bearer was designed
	// or selected to have that disposition.
	// Examples: the function of a pump to pump, the function of a sensor to sense
	Function = Namespace + "BFO_0000034"
)

// Occurrent Subtypes
//
// Processes, boundaries, and temporal/spatiotemporal regions.
const (
	// Process is an occurrent that unfolds in time.
	// Examples: a flight mission, a sensor reading, a communication
	Process = Namespace + "BFO_0000015"

	// ProcessBoundary is the instantaneous beginning or ending of a process.
	ProcessBoundary = Namespace + "BFO_0000035"

	// SpatiotemporalRegion is an occurrent that is part of spacetime.
	SpatiotemporalRegion = Namespace + "BFO_0000011"

	// TemporalRegion is an occurrent that is part of time.
	TemporalRegion = Namespace + "BFO_0000008"

	// ZeroDimensionalTemporalRegion is an instant in time.
	ZeroDimensionalTemporalRegion = Namespace + "BFO_0000148"

	// OneDimensionalTemporalRegion is an interval of time.
	OneDimensionalTemporalRegion = Namespace + "BFO_0000038"

	// History is the sum of all processes that have a given continuant as participant.
	History = Namespace + "BFO_0000182"
)

// Core Relations
//
// Fundamental BFO relations for expressing relationships between entities.
const (
	// PartOf relates a part to its whole.
	// Domain: Entity, Range: Entity
	// Example: wing partOf drone
	PartOf = Namespace + "BFO_0000050"

	// HasPart relates a whole to its part (inverse of PartOf).
	// Domain: Entity, Range: Entity
	// Example: drone hasPart wing
	HasPart = Namespace + "BFO_0000051"

	// OccursIn relates a process to the site where it occurs.
	// Domain: Process, Range: Site
	// Example: mission occursIn airspace
	OccursIn = Namespace + "BFO_0000066"

	// ParticipatesIn relates a continuant to a process it participates in.
	// Domain: Continuant, Range: Process
	// Example: drone participatesIn mission
	ParticipatesIn = Namespace + "BFO_0000056"

	// HasParticipant relates a process to a continuant that participates (inverse of ParticipatesIn).
	// Domain: Process, Range: Continuant
	// Example: mission hasParticipant drone
	HasParticipant = Namespace + "BFO_0000057"

	// RealizedIn relates a realizable entity to a process that realizes it.
	// Domain: RealizableEntity, Range: Process
	// Example: pilotRole realizedIn flyingProcess
	RealizedIn = Namespace + "BFO_0000054"

	// Realizes relates a process to the realizable entity it realizes (inverse of RealizedIn).
	// Domain: Process, Range: RealizableEntity
	// Example: flyingProcess realizes pilotRole
	Realizes = Namespace + "BFO_0000055"

	// BearerOf relates an independent continuant to a dependent continuant it bears.
	// Domain: IndependentContinuant, Range: SpecificallyDependentContinuant
	// Example: drone bearerOf batteryLevel
	BearerOf = Namespace + "BFO_0000053"

	// InheresIn relates a specifically dependent continuant to its bearer (inverse of BearerOf).
	// Domain: SpecificallyDependentContinuant, Range: IndependentContinuant
	// Example: batteryLevel inheresIn drone
	InheresIn = Namespace + "BFO_0000052"

	// LocatedIn relates a continuant to its containing spatial region.
	// Domain: IndependentContinuant, Range: SpatialRegion
	LocatedIn = Namespace + "BFO_0000171"

	// LocationOf relates a spatial region to what is located there (inverse of LocatedIn).
	// Domain: SpatialRegion, Range: IndependentContinuant
	LocationOf = Namespace + "BFO_0000124"

	// OccupiesSpatialRegion relates a material entity to the region it occupies.
	// Domain: MaterialEntity, Range: SpatialRegion
	OccupiesSpatialRegion = Namespace + "BFO_0000083"
)

// Temporal Relations
//
// Relations for expressing temporal ordering and existence.
const (
	// ExistsAt relates a continuant to a temporal region at which it exists.
	// Domain: Continuant, Range: TemporalRegion
	ExistsAt = Namespace + "BFO_0000108"

	// PrecedesTemporally relates one occurrent to another that follows it in time.
	// Domain: Occurrent, Range: Occurrent
	// Example: takeoff precedesTemporally cruising
	PrecedesTemporally = Namespace + "BFO_0000062"

	// TemporalPartOf relates a temporal part to the whole occurrent.
	// Domain: Occurrent, Range: Occurrent
	TemporalPartOf = Namespace + "BFO_0000139"

	// HasTemporalPart relates an occurrent to its temporal parts (inverse of TemporalPartOf).
	// Domain: Occurrent, Range: Occurrent
	HasTemporalPart = Namespace + "BFO_0000121"

	// OccupiesTemporalRegion relates a process to the temporal region it occupies.
	// Domain: Process, Range: TemporalRegion
	OccupiesTemporalRegion = Namespace + "BFO_0000132"
)

// Spatial Relations
//
// Additional relations for spatial reasoning.
const (
	// ContainedIn relates an independent continuant to a site that contains it.
	// Domain: IndependentContinuant, Range: Site
	// Example: drone containedIn hangar
	ContainedIn = Namespace + "BFO_0000170"

	// Contains relates a site to what it contains (inverse of ContainedIn).
	// Domain: Site, Range: IndependentContinuant
	// Example: hangar contains drone
	Contains = Namespace + "BFO_0000169"

	// AdjacentTo relates spatial regions that share a boundary.
	// Domain: SpatialRegion, Range: SpatialRegion
	AdjacentTo = Namespace + "BFO_0000068"
)
