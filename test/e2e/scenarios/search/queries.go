package search

// DefaultQueries returns the standard search quality test queries.
// These cover natural language searches and known-answer validation.
func DefaultQueries() []Query {
	return []Query{
		// Natural language tests
		{
			Text:            "What documents mention forklift safety?",
			ExpectedPattern: "forklift",
			Description:     "Natural language document search",
			MinScore:        0.3,
			MinHits:         1,
			MustInclude:     []string{"doc-ops-001"}, // Forklift Operation Manual
			MustExclude:     []string{"sensor-temp"}, // Temperature sensors irrelevant
		},
		{
			Text:            "Are there safety observations related to temperature?",
			ExpectedPattern: "temperature",
			Description:     "Cross-domain safety query",
			MinScore:        0.3,
			MinHits:         1,
			MustInclude:     []string{"sensor-temp"}, // Temperature sensors
		},
		{
			Text:            "What maintenance was done on cold storage equipment?",
			ExpectedPattern: "cold",
			Description:     "Maintenance semantic search",
			MinScore:        0.3,
			MinHits:         1,
			MustInclude:     []string{"maint-"}, // Maintenance records
		},
		{
			Text:            "Find all sensors in zone-a",
			ExpectedPattern: "zone-a",
			Description:     "Location-based sensor query",
			MinScore:        0.3,
			MinHits:         1,
		},
		// Known-answer tests derived from testdata/semantic/
		{
			Text:            "forklift operation inspection equipment maintenance",
			ExpectedPattern: "ops",
			Description:     "Operations query should return operations docs",
			MinScore:        0.3,
			MinHits:         1,
			MustInclude:     []string{"doc-ops"},                       // doc-ops-001 (Forklift Operation Manual)
			MustExclude:     []string{"sensor-humid", "sensor-motion"}, // Humidity/motion sensors irrelevant
		},
		{
			Text:            "cold storage temperature monitoring refrigeration",
			ExpectedPattern: "temp",
			Description:     "Temperature query should return temp sensors",
			MinScore:        0.3,
			MinHits:         1,
			MustInclude:     []string{"sensor-temp"},         // sensor-temp-001, sensor-temp-002, etc.
			MustExclude:     []string{"doc-hr", "doc-audit"}, // HR and audit docs irrelevant
		},
		{
			Text:            "hydraulic fluid maintenance equipment repair",
			ExpectedPattern: "maint",
			Description:     "Maintenance query should return maintenance records",
			MinScore:        0.3,
			MinHits:         1,
			MustInclude:     []string{"maint-"}, // maint-001 (hydraulic maintenance)
		},
	}
}

// StructuralQueries returns queries appropriate for structural tier (no embeddings).
// These should all return zero results since structural tier has no semantic search.
func StructuralQueries() []Query {
	return []Query{
		{
			Text:        "forklift safety",
			Description: "Structural tier should return no semantic results",
			MinScore:    0,
			MinHits:     0, // Expect zero hits
		},
	}
}
