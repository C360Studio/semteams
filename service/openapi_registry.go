// Package service provides the OpenAPI registry for service specifications
package service

import "sync"

// openAPIRegistry holds all registered service OpenAPI specifications
var (
	openAPIRegistry   = make(map[string]*OpenAPISpec)
	openAPIRegistryMu sync.RWMutex
)

// RegisterOpenAPISpec registers an OpenAPI specification for a service.
// This should be called from init() functions in service files.
func RegisterOpenAPISpec(name string, spec *OpenAPISpec) {
	openAPIRegistryMu.Lock()
	defer openAPIRegistryMu.Unlock()
	openAPIRegistry[name] = spec
}

// GetAllOpenAPISpecs returns all registered OpenAPI specifications.
// Used by the openapi-generator tool to collect specs from all services.
func GetAllOpenAPISpecs() map[string]*OpenAPISpec {
	openAPIRegistryMu.RLock()
	defer openAPIRegistryMu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*OpenAPISpec, len(openAPIRegistry))
	for k, v := range openAPIRegistry {
		result[k] = v
	}
	return result
}

// GetOpenAPISpec returns the OpenAPI specification for a specific service.
func GetOpenAPISpec(name string) (*OpenAPISpec, bool) {
	openAPIRegistryMu.RLock()
	defer openAPIRegistryMu.RUnlock()
	spec, ok := openAPIRegistry[name]
	return spec, ok
}
