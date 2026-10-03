package mcp

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// mustEnumInputSchema infers a tool's input schema and pins the value sets the jsonschema
// tag cannot express. An enum keeps the values discrete in the schema, where a
// comma-separated description hands clients a sentence they may render as one run-on word.
//
// A failure here is a bug in a tool's own types, so it panics at registration the way the
// MCP SDK's AddTool does.
func mustEnumInputSchema[T any](enums map[string][]string) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("infer input schema: %v", err))
	}

	for property, allowed := range enums {
		field, ok := schema.Properties[property]
		if !ok {
			panic(fmt.Sprintf("inferred schema has no %q property", property))
		}

		field.Enum = make([]any, 0, len(allowed))
		for _, value := range allowed {
			field.Enum = append(field.Enum, value)
		}
	}

	return schema
}
