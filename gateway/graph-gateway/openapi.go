package graphgateway

import (
	"reflect"

	"github.com/c360studio/semstreams/graph/inference"
	"github.com/c360studio/semstreams/service"
)

func init() {
	service.RegisterOpenAPISpec("graph-gateway", graphGatewayOpenAPISpec())
}

// OpenAPISpec implements gateway.OpenAPIProvider interface.
func (c *Component) OpenAPISpec() *service.OpenAPISpec {
	return graphGatewayOpenAPISpec()
}

// graphGatewayOpenAPISpec returns the OpenAPI specification for graph-gateway endpoints.
func graphGatewayOpenAPISpec() *service.OpenAPISpec {
	return &service.OpenAPISpec{
		Tags: []service.TagSpec{
			{Name: "GraphQL", Description: "GraphQL query endpoint for knowledge graph operations"},
			{Name: "MCP", Description: "Model Context Protocol endpoint for AI tool integration"},
			{Name: "Inference", Description: "Structural anomaly inference review API"},
		},
		Paths: map[string]service.PathSpec{
			"/graphql": {
				POST: &service.OperationSpec{
					Summary:     "Execute GraphQL query",
					Description: "Execute GraphQL queries against the knowledge graph. The GraphQL schema is available via introspection query.",
					Tags:        []string{"GraphQL"},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "GraphQL response with data or errors",
							ContentType: "application/json",
						},
						"400": {Description: "Invalid GraphQL query"},
						"405": {Description: "Method not allowed (only POST supported)"},
						"504": {Description: "Query timeout"},
					},
				},
			},
			"/mcp": {
				POST: &service.OperationSpec{
					Summary:     "MCP endpoint",
					Description: "Model Context Protocol endpoint for AI tool integration. Enables LLMs to interact with the knowledge graph.",
					Tags:        []string{"MCP"},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "MCP response",
							ContentType: "application/json",
						},
					},
				},
			},
			"/inference/anomalies/pending": {
				GET: &service.OperationSpec{
					Summary:     "List pending anomalies",
					Description: "Returns structural anomalies awaiting human review",
					Tags:        []string{"Inference"},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "List of pending anomalies",
							ContentType: "application/json",
							SchemaRef:   "#/components/schemas/StructuralAnomaly",
							IsArray:     true,
						},
						"500": {Description: "Internal server error"},
					},
				},
			},
			"/inference/anomalies/{id}": {
				GET: &service.OperationSpec{
					Summary:     "Get anomaly by ID",
					Description: "Returns details of a specific structural anomaly",
					Tags:        []string{"Inference"},
					Parameters: []service.ParameterSpec{
						{
							Name:        "id",
							In:          "path",
							Required:    true,
							Description: "Anomaly ID",
							Schema:      service.Schema{Type: "string"},
						},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "Anomaly details",
							ContentType: "application/json",
							SchemaRef:   "#/components/schemas/StructuralAnomaly",
						},
						"404": {Description: "Anomaly not found"},
						"500": {Description: "Internal server error"},
					},
				},
			},
			"/inference/anomalies/{id}/review": {
				POST: &service.OperationSpec{
					Summary:     "Submit review decision",
					Description: "Submit a human review decision (approve or reject) for a structural anomaly",
					Tags:        []string{"Inference"},
					Parameters: []service.ParameterSpec{
						{
							Name:        "id",
							In:          "path",
							Required:    true,
							Description: "Anomaly ID",
							Schema:      service.Schema{Type: "string"},
						},
					},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "Updated anomaly after review",
							ContentType: "application/json",
							SchemaRef:   "#/components/schemas/StructuralAnomaly",
						},
						"400": {Description: "Invalid request (bad decision or missing target entity)"},
						"404": {Description: "Anomaly not found"},
						"409": {Description: "Anomaly not in reviewable state"},
						"500": {Description: "Internal server error"},
					},
				},
			},
			"/inference/stats": {
				GET: &service.OperationSpec{
					Summary:     "Get inference statistics",
					Description: "Returns statistics about detected anomalies and their review status",
					Tags:        []string{"Inference"},
					Responses: map[string]service.ResponseSpec{
						"200": {
							Description: "Inference statistics",
							ContentType: "application/json",
							SchemaRef:   "#/components/schemas/StatsResponse",
						},
						"500": {Description: "Internal server error"},
					},
				},
			},
		},
		ResponseTypes: []reflect.Type{
			reflect.TypeOf(inference.StructuralAnomaly{}),
			reflect.TypeOf(inference.StatsResponse{}),
		},
	}
}
