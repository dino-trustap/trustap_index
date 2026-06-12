package swagger

type Spec struct {
	Paths       map[string]map[string]*Endpoint `yaml:"paths"`
	Definitions map[string]*Schema              `yaml:"definitions"`
}

type Endpoint struct {
	OperationID string `yaml:"operationId"`

	Responses  map[int]*Response `yaml:"responses"`
	Parameters []*Parameter      `yaml:"parameters"`

	// TODO We should ideally decouple this from the Swagger endpoint
	// definition, but we take this approach for now for simplicity.
	Server *ServerEndpoint `yaml:"x-toc-endpoint"`
}

type Response struct {
	Schema *Schema `yaml:"schema"`
}

type Schema struct {
	Type                 *SwaggerType       `yaml:"type"`
	Items                *Schema            `yaml:"items"`
	Ref                  *string            `yaml:"$ref"`
	Required             []string           `yaml:"required"`
	Format               *string            `yaml:"format"`
	Properties           map[string]*Schema `yaml:"properties"`
	Enum                 []string           `yaml:"enum"`
	AdditionalProperties *bool              `yaml:"additionalProperties"`

	MinLength *int `yaml:"minLength"`
	Minimum   *int `yaml:"minimum"`
	Maximum   *int `yaml:"maximum"`

	IsNullable *bool `yaml:"x-nullable"`

	// `Metadata` is used to allow consumers to add extra information to the
	// nodes of a Swagger specification without needing to define a custom
	// type to store information within an identical structure.
	Metadata any
}

func (schema *Schema) RequiresProp(propName string) bool {
	return stringsContain(schema.Required, propName)
}

func stringsContain(xs []string, y string) bool {
	for _, x := range xs {
		if x == y {
			return true
		}
	}
	return false
}

type Parameter struct {
	In       string       `yaml:"in"`
	Name     string       `yaml:"name"`
	Required bool         `yaml:"required"`
	Type     *SwaggerType `yaml:"type"`
	Format   *string      `yaml:"format"`
	Schema   *Schema      `yaml:"schema"`
}

// We call this `SwaggerType` instead of `Type` to avoid conflict with the
// built-in type (<https://revive.run/r#redefines-builtin-id>).
type SwaggerType string
