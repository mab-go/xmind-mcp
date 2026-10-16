package handler

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

// outputTypeSchemas overrides the schema jsonschema.For infers for types it cannot describe:
//
//   - xmind.Marker keeps unknown keys from the file and writes them back (json_codec.go), so
//     its schema allows additional properties instead of the inferred false.
//   - SubtreeNode is recursive, which jsonschema.For rejects. Its children refer back to the
//     root of the output schema ("#"), which is the node schema itself.
var outputTypeSchemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[xmind.Marker](): {
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{"markerId": {Type: "string"}},
		Required:   []string{"markerId"},
	},
	reflect.TypeFor[[]*SubtreeNode](): {
		Type:  "array",
		Items: &jsonschema.Schema{Ref: "#"},
	},
}

// OutputSchema returns a tool option declaring T's JSON Schema as the tool's outputSchema.
// T is the struct a handler passes to jsonResult. Unlike mcp.WithOutputSchema, which logs a
// schema error to stderr and registers no schema, OutputSchema panics: tools are package-level
// vars, so a bad schema fails every test and stops the binary at startup.
func OutputSchema[T any]() mcp.ToolOption {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: outputTypeSchemas})
	if err != nil {
		panic(fmt.Sprintf("output schema for %v: %v", reflect.TypeFor[T](), err))
	}
	if schema.Type != "object" {
		panic(fmt.Sprintf("output schema for %v: type %q, want \"object\"", reflect.TypeFor[T](), schema.Type))
	}
	data, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("output schema for %v: %v", reflect.TypeFor[T](), err))
	}
	return mcp.WithRawOutputSchema(data)
}
