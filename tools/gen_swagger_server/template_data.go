package main

import (
	"github.com/trustap/rest_api/tools/gen_swagger_server/swagger"
)

type goType string

type snippet string

type endpoint struct {
	Name *name

	Method *name
	Path   string
	Meta   *endpointMeta

	Context         *endpointContext
	Params          *paramGroups
	SuccessResponse *endpointResponse
}

type endpointMeta struct {
	GoType goType
	Values map[string]snippet
}

type endpointContext struct {
	Type   goType
	TypeID *name
}

type paramGroups struct {
	OptionalBodyType *goType
	Path             []*param
	Query            []*param
}

type param struct {
	Name       *name
	GoType     string
	IsRequired bool
	FuncSuffix snippet
}

type endpointResponse struct {
	Status         *name
	OptionalGoType *goType
}

type definition struct {
	Name                      *name
	SwaggerType               swagger.SwaggerType
	Enum                      []*name
	Properties                []*definitionProperty
	AllowAdditionalProperties bool

	UseCustomUnmarshalJSON bool
}

// `definitionsByName` is used for sorting a slice of `definition`s using
// `sort.Sort`.
type definitionsByName []*definition

func (ps definitionsByName) Len() int {
	return len(ps)
}

func (ps definitionsByName) Swap(i, j int) {
	ps[i], ps[j] = ps[j], ps[i]
}

func (ps definitionsByName) Less(i, j int) bool {
	return ps[i].Name.Camel() < ps[j].Name.Camel()
}

type definitionProperty struct {
	Name       *name
	GoType     goType
	IsRequired bool
	SwaggerTag string
}

// `definitionPropertiesByName` is used for sorting a slice of
// `definitionProperty` using `sort.Sort`.
type definitionPropertiesByName []*definitionProperty

func (ps definitionPropertiesByName) Len() int {
	return len(ps)
}

func (ps definitionPropertiesByName) Swap(i, j int) {
	ps[i], ps[j] = ps[j], ps[i]
}

func (ps definitionPropertiesByName) Less(i, j int) bool {
	return ps[i].Name.Camel() < ps[j].Name.Camel()
}
