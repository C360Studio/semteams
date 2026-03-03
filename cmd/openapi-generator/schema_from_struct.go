package main

import (
	"reflect"

	"github.com/c360studio/semstreams/service"
)

// schemaFromType delegates to service.SchemaFromType for JSON Schema generation.
func schemaFromType(t reflect.Type) map[string]any {
	return service.SchemaFromType(t)
}

// typeNameFromReflect delegates to service.TypeNameFromReflect for type name extraction.
func typeNameFromReflect(t reflect.Type) string {
	return service.TypeNameFromReflect(t)
}
