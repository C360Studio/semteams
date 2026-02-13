package vocab

// Well-known RDF namespace prefixes used in TrustGraph and semantic web applications.
const (
	// RDF is the W3C RDF syntax namespace.
	// Common predicates: rdf:type
	RDF = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"

	// RDFS is the W3C RDF Schema namespace.
	// Common predicates: rdfs:label, rdfs:comment, rdfs:subClassOf
	RDFS = "http://www.w3.org/2000/01/rdf-schema#"

	// SOSA is the W3C Sensor, Observation, Sample, and Actuator ontology.
	// Common predicates: sosa:observes, sosa:hasSimpleResult, sosa:resultTime, sosa:isHostedBy
	SOSA = "http://www.w3.org/ns/sosa/"

	// SSN is the W3C Semantic Sensor Network ontology (extends SOSA).
	// Common predicates: ssn:hasProperty, ssn:forProperty
	SSN = "http://www.w3.org/ns/ssn/"

	// BFO is the Basic Formal Ontology (OBO Foundry).
	// Common predicates: BFO_0000050 (part_of), BFO_0000129 (member_of)
	BFO = "http://purl.obolibrary.org/obo/"

	// GEO is the OGC GeoSPARQL namespace.
	// Common predicates: geo:hasGeometry, geo:asWKT
	GEO = "http://www.opengis.net/ont/geosparql#"

	// SCHEMA is the Schema.org namespace.
	// Common predicates: schema:name, schema:description, schema:location
	SCHEMA = "http://schema.org/"

	// TG is the TrustGraph default entity namespace.
	TG = "http://trustgraph.ai/e/"

	// XSD is the XML Schema datatypes namespace.
	// Common datatypes: xsd:string, xsd:integer, xsd:float, xsd:dateTime, xsd:boolean
	XSD = "http://www.w3.org/2001/XMLSchema#"

	// OWL is the W3C Web Ontology Language namespace.
	// Common predicates: owl:sameAs, owl:differentFrom
	OWL = "http://www.w3.org/2002/07/owl#"

	// DC is the Dublin Core metadata namespace.
	// Common predicates: dc:title, dc:creator, dc:date
	DC = "http://purl.org/dc/elements/1.1/"

	// DCTERMS is the Dublin Core Terms namespace (extended vocabulary).
	// Common predicates: dcterms:created, dcterms:modified, dcterms:subject
	DCTERMS = "http://purl.org/dc/terms/"

	// SKOS is the Simple Knowledge Organization System namespace.
	// Common predicates: skos:prefLabel, skos:altLabel, skos:broader, skos:narrower
	SKOS = "http://www.w3.org/2004/02/skos/core#"
)

// Namespaces maps short prefixes to full namespace URIs.
// Used for compact URI (CURIE) expansion.
var Namespaces = map[string]string{
	"rdf":     RDF,
	"rdfs":    RDFS,
	"sosa":    SOSA,
	"ssn":     SSN,
	"bfo":     BFO,
	"geo":     GEO,
	"schema":  SCHEMA,
	"tg":      TG,
	"xsd":     XSD,
	"owl":     OWL,
	"dc":      DC,
	"dcterms": DCTERMS,
	"skos":    SKOS,
}

// WellKnownPredicates maps common SemStreams predicates to their RDF equivalents.
// This provides a default set of mappings that can be extended via configuration.
var WellKnownPredicates = map[string]string{
	// Classification
	"entity.classification.type": RDF + "type",

	// Metadata
	"entity.metadata.label":       RDFS + "label",
	"entity.metadata.comment":     RDFS + "comment",
	"entity.metadata.description": SCHEMA + "description",
	"entity.metadata.name":        SCHEMA + "name",

	// Sensor observations (SOSA)
	"sensor.measurement.value":    SOSA + "hasSimpleResult",
	"sensor.measurement.celsius":  SOSA + "hasSimpleResult",
	"sensor.observation.time":     SOSA + "resultTime",
	"sensor.observation.property": SOSA + "observes",

	// Location (SOSA + GeoSPARQL)
	"geo.location.zone":     SOSA + "isHostedBy",
	"geo.location.geometry": GEO + "hasGeometry",

	// Relationships (BFO)
	"relation.part_of":   BFO + "BFO_0000050",
	"relation.member_of": BFO + "BFO_0000129",

	// Generic relations
	"relation.relates_to": SCHEMA + "relatedTo",
	"relation.depends_on": SCHEMA + "isBasedOn",

	// Temporal
	"temporal.created":  DCTERMS + "created",
	"temporal.modified": DCTERMS + "modified",
}
