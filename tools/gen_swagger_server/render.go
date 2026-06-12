package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"4d63.com/optional"
	"github.com/trustap/rest_api/pkg/hashset"
	"github.com/trustap/rest_api/pkg/ptr"
	"github.com/trustap/rest_api/tools/gen_swagger_server/swagger"
)

func renderFiles(
	templs *template.Template,
	config *config,
	swaggerSpec *swagger.Spec,
) (fileLayout, error) {
	splitter := &refNameSplitter{defaultSvcID: config.DefaultService}
	endpts := flattenPaths(swaggerSpec.Paths)
	defns := map[string]*swagger.Schema{}
	declaredDefns := hashset.NewSet[string]()
	for defnName, defn := range swaggerSpec.Definitions {
		ref, err := splitter.splitRefName(defnName, newNameFromPascal)
		if err != nil {
			return nil, fmt.Errorf("couldn't split definition name '%s': %w", defnName, err)
		}
		name := ref.CanonName()
		defns[name] = defn
		declaredDefns.Set(name)
	}

	err := setInlineDefnNamesOnEndpts(splitter, endpts)
	if err != nil {
		return nil, fmt.Errorf("couldn't set inline definition metadata on endpoints: %w", err)
	}
	err = setInlineDefnNamesOnDefinitions(splitter, defns)
	if err != nil {
		return nil, fmt.Errorf("couldn't set inline definition metadata on definitions: %w", err)
	}

	for defnName, defn := range extractInlineDefinitionsFromSwaggerDefinitions(defns) {
		// TODO Check for name collisions.
		defns[defnName] = defn
	}
	for defnName, defn := range extractInlineDefinitionsFromSwaggerPaths(endpts) {
		// TODO Check for name collisions.
		defns[defnName] = defn
	}

	// TODO Consider whether inline definitions need to be included in
	// `refs`.
	refs := newRefsFromSwaggerDefinitions(splitter, defns)
	files, err := renderModelFiles(refs, templs, splitter, defns, declaredDefns)
	if err != nil {
		return nil, fmt.Errorf("couldn't render definitions files: %w", err)
	}

	fs, err := renderServerFiles(templs, config)
	if err != nil {
		return nil, fmt.Errorf("couldn't render server files: %w", err)
	}

	files, err = mergeFileLayouts(files, fs)
	if err != nil {
		return nil, fmt.Errorf("couldn't merge server files: %w", err)
	}

	fs, err = renderAPIFiles(templs, config, refs, splitter, endpts)
	if err != nil {
		return nil, fmt.Errorf("couldn't render API files: %w", err)
	}

	files, err = mergeFileLayouts(files, fs)
	if err != nil {
		return nil, fmt.Errorf("couldn't merge API files: %w", err)
	}

	return files, nil
}

func flattenPaths(paths map[string]map[string]*swagger.Endpoint) []*swaggerEndpoint {
	endpts := []*swaggerEndpoint{}
	for path, methods := range paths {
		for method, endpt := range methods {
			ep := &swaggerEndpoint{
				Endpoint: endpt,
				method:   method,
				path:     path,
			}
			endpts = append(endpts, ep)
		}
	}
	return endpts
}

type swaggerEndpoint struct {
	*swagger.Endpoint

	method string
	path   string
}

type fileLayout map[string]snippet

// `mergeFileLayouts` returns a new `fileLayout` with the combined contents of
// `a` and `b`.
func mergeFileLayouts(a, b fileLayout) (fileLayout, error) {
	c := fileLayout{}

	for k, v := range a {
		c[k] = v
	}
	for k, v := range b {
		_, exists := c[k]
		if exists {
			return nil, fmt.Errorf("both maps contain '%s'", k)
		}

		c[k] = v
	}

	return c, nil
}

func renderTemplate(t *template.Template, data any) (snippet, error) {
	buf := &bytes.Buffer{}
	err := t.Execute(buf, data)
	if err != nil {
		return "", fmt.Errorf("couldn't execute template: %w", err)
	}
	return snippet(buf.String()), nil
}

func renderModelFiles(
	refs *refs,
	templs *template.Template,
	splitter *refNameSplitter,
	definitions map[string]*swagger.Schema,
	declaredDefns hashset.Set[string],
) (fileLayout, error) {
	svcsDefns := map[string]*defnSets{}

	for defnName, defn := range definitions {
		enum := []*name{}
		props := []*definitionProperty{}

		ref, err := splitter.splitRefName(defnName, newNameFromPascal)
		if err != nil {
			return nil, fmt.Errorf("couldn't split referenced definition '%s': %w", defnName, err)
		}

		if defn.Ref != nil {
			// TODO Add support for `$ref`s in definitions.
			return nil, fmt.Errorf("'$ref' aren't currently supported in definitions ('%s'): %w", defnName, err)
		}

		if defn.Type == nil {
			return nil, fmt.Errorf("definition '%s' doesn't contain 'type'", defnName)
		}
		defnType := *defn.Type

		switch defnType {
		case "string":
			for _, e := range defn.Enum {
				enum = append(enum, newNameFromSnake(e))
			}
		case "object":
			for rawPropName, prop := range defn.Properties {
				propName := newNameFromSnake(rawPropName)

				nestedDefnName := ref.Name.Pascal() + propName.Pascal()
				goType, err := calcPropertyGoType(refs, ref.SvcID, nestedDefnName, prop)
				if err != nil {
					return nil, fmt.Errorf("couldn't extract Go type for property '%s.%s': %w", ref.Name.Pascal(), rawPropName, err)
				}

				swaggerType, err := getSchemaType(refs, prop)
				if err != nil {
					return nil, fmt.Errorf("couldn't get Swagger type for property '%s.%s': %w", ref.Name.Pascal(), rawPropName, err)
				}

				isRequired := defn.RequiresProp(rawPropName)
				isNullable := prop.IsNullable != nil && *prop.IsNullable
				if (!isRequired && !isNullable && swaggerType != "array") || swaggerType == "object" {
					goType = "*" + goType
				}

				if isNullable {
					if isRequired {
						return nil, fmt.Errorf("required fields can't be nullable at present; instead, make the field non-required")
					}
					goType = "swagger_rest.Optional[" + goType + "]"
				}

				swaggerTags := []string{}
				if isRequired {
					swaggerTags = append(swaggerTags, "required")
				}

				if prop.MinLength != nil {
					t := fmt.Sprintf("min_length:%d", *prop.MinLength)
					swaggerTags = append(swaggerTags, t)
				}
				if prop.Minimum != nil {
					t := fmt.Sprintf("minimum:%d", *prop.Minimum)
					swaggerTags = append(swaggerTags, t)
				}
				if prop.Maximum != nil {
					t := fmt.Sprintf("maximum:%d", *prop.Maximum)
					swaggerTags = append(swaggerTags, t)
				}

				p := &definitionProperty{
					Name:       propName,
					GoType:     goType,
					IsRequired: isRequired,
					SwaggerTag: strings.Join(swaggerTags, ","),
				}
				props = append(props, p)
			}
		default:
			return nil, fmt.Errorf("unsupported definition type '%s'", defnType)
		}

		sort.Sort(definitionPropertiesByName(props))
		var d *definition

		allowAdditionalProperties := false
		if defn.AdditionalProperties != nil {
			allowAdditionalProperties = *defn.AdditionalProperties
		}

		d = &definition{
			Name:                      ref.Name,
			SwaggerType:               defnType,
			Enum:                      enum,
			Properties:                props,
			AllowAdditionalProperties: allowAdditionalProperties,
		}

		defns, ok := svcsDefns[ref.SvcID]
		if !ok {
			defns = &defnSets{
				declaredDefns: []*definition{},
				inlineDefns:   []*definition{},
			}
		}
		svcsDefns[ref.SvcID] = defns

		if declaredDefns.Has(ref.CanonName()) {
			defns.declaredDefns = append(defns.declaredDefns, d)
		} else {
			defns.inlineDefns = append(defns.inlineDefns, d)
		}
	}

	layout := fileLayout{}
	for svcID, defnSets := range svcsDefns {
		sets := map[string][]*definition{
			"models":        defnSets.declaredDefns,
			"inline_models": defnSets.inlineDefns,
		}

		for baseName, defns := range sets {
			if len(defns) == 0 {
				continue
			}

			sort.Sort(definitionsByName(defns))

			data := map[string]any{
				"Pkg":         svcID,
				"Definitions": defns,
			}
			f, err := renderTemplate(templs.Lookup("models.gotempl"), data)
			if err != nil {
				return nil, fmt.Errorf("couldn't render template: %w", err)
			}
			layout[svcID+"/"+baseName+".go"] = f
		}
	}
	return layout, nil
}

type defnSets struct {
	declaredDefns []*definition
	inlineDefns   []*definition
}

func getSchemaType(refs *refs, schema *swagger.Schema) (swagger.SwaggerType, error) {
	if schema.Type != nil {
		return *schema.Type, nil
	} else if schema.Ref != nil {
		ref := *schema.Ref
		schemaRef, err := refs.LookupSchemaRef(ref)
		if err != nil {
			return "", fmt.Errorf("couldn't lookup reference '%s': %w", ref, err)
		}

		t, err := getSchemaType(refs, schemaRef.schema)
		if err != nil {
			return "", fmt.Errorf("couldn't get schema type for '%s': %w", ref, err)
		}
		return t, nil
	}

	return "", fmt.Errorf("schema doesn't define type or reference")
}

// `calcPropertyGoType` returns the Go type associated with `prop`.
func calcPropertyGoType(refs *refs, svcID string, name string, prop *swagger.Schema) (goType, error) {
	if prop.Type != nil {
		swaggerType := *prop.Type

		if swaggerType == "object" {
			return goType(name), nil
		}

		if swaggerType == "string" && prop.Enum != nil {
			typ, err := swaggerSchemaToGoType(refs, svcID, prop)
			if err != nil {
				return "", fmt.Errorf("couldn't convert Swagger schema to Go type: %w", err)
			}
			return typ, nil
		}

		if swaggerType == "array" {
			// TODO We handle one specific array type for now, but
			// this block should be updated to process the type
			// recursively.

			if prop.Items == nil {
				return "", fmt.Errorf("array type didn't contain associated `items`")
			}
			items := prop.Items

			typ, err := swaggerSchemaToGoType(refs, svcID, items)
			if err != nil {
				return "", fmt.Errorf("couldn't convert Swagger schema to Go type: %w", err)
			}

			return goType("[]") + typ, nil
		}

		goType, err := basicTypeSwaggerToGo(swaggerType, optional.OfPtr[string](prop.Format))
		if err != nil {
			return "", fmt.Errorf("couldn't convert basic Swagger type to Go type: %w", err)
		}
		return goType, nil
	} else if prop.Ref != nil {
		ref, err := refs.LookupSchemaRef(*prop.Ref)
		if err != nil {
			return "", fmt.Errorf("couldn't convert Swagger reference to Go type: %w", err)
		}
		return ref.ToGoType(svcID), nil
	}

	return "", fmt.Errorf("property doesn't define type or reference")
}

// `basicTypeSwaggerToGo` returns the Go type corresponding to the `swaggerType`
// and `format`.
//
// Note that this function can only convert types that correspond to Go types;
// if `swaggerType` is `object`, for example, this indicates a custom type that
// this function doesn't support.
func basicTypeSwaggerToGo(swaggerType swagger.SwaggerType, format optional.Optional[string]) (goType, error) {
	switch swaggerType {
	case "integer":
		return "int", nil
	case "number":
		f, ok := format.Get()
		if !ok {
			return "", fmt.Errorf("'number' without a format is currently unsupported")
		}

		if f == "float" {
			return "float32", nil
		} else if f == "double" {
			return "float64", nil
		}
		return "", fmt.Errorf("'%s' is not currently supported as a 'number' format", f)
	case "boolean":
		return "bool", nil
	case "string":
		f, ok := format.Get()
		if !ok {
			return "string", nil
		}

		if f == "date-time" {
			return "time.Time", nil
		}
		// According to the Swagger specification:
		//
		// > Tools that do not support a specific format may default
		// > back to the `type` alone, as if the `format` is not
		// > specified.
		return "string", nil
	default:
		return "", fmt.Errorf("the Swagger type '%s' doesn't correspond to any Go type", swaggerType)
	}
}

func renderServerFiles(templs *template.Template, config *config) (fileLayout, error) {
	imports := map[string]*string{
		"net/http": nil,
		// TODO Allow import aliases.
		config.ContextTypes.Import:     nil,
		config.EndpointMetadata.Import: nil,
	}

	// TODO Generate specific refiner for service.
	localContextTypes := localContextTypesRawToGoTypes(config.ContextTypes.Local)

	lcts := map[string]goType{}
	for name, typ := range localContextTypes {
		namePascal := newNameFromSnake(name).Pascal()
		lcts[namePascal] = typ
	}

	data := &endpointTemplFields{
		Imports:              imports,
		ServerPackageName:    config.GoPackage.Name,
		EndpointMetadataType: goType(config.EndpointMetadata.Type),
		GlobalContextType:    goType(config.ContextTypes.Global),
		LocalContextTypes:    lcts,
	}
	f, err := renderTemplate(templs.Lookup("endpoint.gotempl"), data)
	if err != nil {
		return nil, fmt.Errorf("couldn't render template 'endpoint.gotempl': %w", err)
	}
	return fileLayout{"endpoint.go": f}, nil
}

type endpointTemplFields struct {
	Imports              map[string]*string
	ServerPackageName    string
	EndpointMetadataType goType
	GlobalContextType    goType
	LocalContextTypes    map[string]goType
}

func renderAPIFiles(
	templs *template.Template,
	config *config,
	refs *refs,
	splitter *refNameSplitter,
	endpts []*swaggerEndpoint,
) (fileLayout, error) {
	// We currently use `goimports` to remove unused imports from the
	// generated files. TODO We should only add imports that are actually
	// used by the different files.
	imports := map[string]*string{
		"encoding/json": nil,
		"fmt":           nil,
		"net/http":      nil,
		// TODO Allow import aliases.
		config.ContextTypes.Import:     nil,
		config.EndpointMetadata.Import: nil,
		config.GoPackage.Path:          &config.GoPackage.Name,

		"github.com/trustap/rest_api/pkg/json":                                   ptr.String("rest_api_json"),
		"github.com/trustap/trustap_index/tools/gen_swagger_server/swagger_rest": nil,
	}

	// TODO Generate specific refiner for service.
	localContextTypes := localContextTypesRawToGoTypes(config.ContextTypes.Local)

	svcsEndpoints := map[string][]*endpoint{}
	for _, endpt := range endpts {
		// TODO Avoid creating a new set on every iteration.
		if config.IncludeEndpoints != nil {
			methods, ok := config.IncludeEndpoints[endpt.path]
			if !ok || !hashset.ImmutableSetFromSlice(methods).Has(endpt.method) {
				continue
			}
		}

		ref, err := splitter.splitRefName(endpt.OperationID, newNameFromCamel)
		if err != nil {
			return nil, fmt.Errorf("couldn't split operation '%s': %w", endpt.OperationID, err)
		}

		params := &paramGroups{OptionalBodyType: nil, Path: []*param{}, Query: []*param{}}
		if endpt.Parameters != nil {
			var err error
			params, err = splitParams(ref.SvcID, ref.Name, refs, endpt.Parameters)
			if err != nil {
				return nil, fmt.Errorf("couldn't split parameters ('%s'): %w", endpt.OperationID, err)
			}
		}

		if endpt.Server == nil {
			// TODO This limitation can be relaxed in future
			// by allowing a default context to be used
			// instead.
			return nil, fmt.Errorf("operation '%s' doesn't contain 'x-toc-endpoint'", endpt.OperationID)
		}
		localContextTypeID := endpt.Server.ContextTypeID

		localContextType, ok := localContextTypes[localContextTypeID]
		if !ok {
			return nil, fmt.Errorf("couldn't get local context type '%s' for endpoint", localContextTypeID)
		}

		ep, err := newEndpointFromSwagger(
			refs,
			ref.SvcID,
			ref.Name,
			endpt,
			params,
			goType(config.EndpointMetadata.Type),
			&endpointContext{
				Type:   localContextType,
				TypeID: newNameFromSnake(localContextTypeID),
			},
		)
		if err != nil {
			msg := "couldn't construct endpoint field for '%s' (%s %s): %w"
			return nil, fmt.Errorf(msg, endpt.OperationID, endpt.method, endpt.path, err)
		}

		svcEndpoints, ok := svcsEndpoints[ref.SvcID]
		if !ok {
			svcEndpoints = []*endpoint{}
		}
		svcsEndpoints[ref.SvcID] = append(svcEndpoints, ep)
	}

	lcts := map[string]goType{}
	for name, typ := range localContextTypes {
		namePascal := newNameFromSnake(name).Pascal()
		lcts[namePascal] = typ
	}

	// TODO We must also import from services that models are defined in.
	for svcID := range svcsEndpoints {
		imports[config.GoPackage.Path+"/"+svcID] = nil
	}

	layout := fileLayout{}
	for svcID, endpoints := range svcsEndpoints {
		data := &apiFile{
			Pkg:                  svcID,
			Imports:              imports,
			ServerPackageName:    config.GoPackage.Name,
			EndpointMetadataType: goType(config.EndpointMetadata.Type),
			GlobalContextType:    goType(config.ContextTypes.Global),
			Endpoints:            endpoints,
			LocalContextTypes:    lcts,
		}

		for _, templName := range apiTemplNames {
			f, err := renderTemplate(templs.Lookup(templName), data)
			if err != nil {
				return nil, fmt.Errorf("couldn't render template '%s': %w", templName, err)
			}
			// Remove extra `templ` in the file name.
			fname := strings.Split(templName, "templ")[0]
			layout[svcID+"/"+fname] = f
		}
	}
	return layout, nil
}

var apiTemplNames = []string{"api.gotempl", "endpoints.gotempl", "endpoint_handlers.gotempl"}

type apiFile struct {
	Pkg                  string
	Imports              map[string]*string
	ServerPackageName    string
	EndpointMetadataType goType
	GlobalContextType    goType

	Endpoints []*endpoint

	LocalContextTypes map[string]goType
}

func getSuccessResponse(refs *refs, svcID string, resps map[int]*swagger.Response) (*endpointResponse, error) {
	var successResp *endpointResponse

	for respCode, resp := range resps {
		if respCode < 200 || respCode >= 300 {
			continue
		}
		if successResp != nil {
			return nil, fmt.Errorf("multiple success responses defined")
		}

		status, ok := respStatuses[respCode]
		if !ok {
			// TODO Any code caught here should have support added.
			return nil, fmt.Errorf("dev err: unsupported success response code: %d", respCode)
		}

		var optionalGoType *goType
		if respCode != 204 {
			goType, err := swaggerSchemaToGoType(refs, svcID, resp.Schema)
			if err != nil {
				return nil, fmt.Errorf("couldn't convert Swagger schema to Go type: %w", err)
			}
			// TODO Move into `swaggerSchemaToGoType`.
			schemaType := resp.Schema.Type
			if schemaType != nil && *schemaType == "array" {
				goType = "[]" + goType
			}
			optionalGoType = &goType
		}

		newResp := &endpointResponse{
			Status:         newNameFromPascal(status),
			OptionalGoType: optionalGoType,
		}

		successResp = newResp
	}

	if successResp == nil {
		return nil, fmt.Errorf("no success response defined")
	}
	return successResp, nil
}

var respStatuses = map[int]string{
	200: "OK",
	202: "Accepted",
	201: "Created",
	204: "NoContent",
}

func swaggerSchemaToGoType(refs *refs, svcID string, schema *swagger.Schema) (goType, error) {
	if schema.Ref != nil {
		ref, err := refs.LookupSchemaRef(*schema.Ref)
		if err != nil {
			return "", fmt.Errorf("couldn't convert Swagger reference to Go type: %w", err)
		}

		typ := ref.ToGoType(svcID)
		// TODO Consider when `ref.schema` would be `nil`, and whether
		// we can remove this check by always ensuring `schema` is
		// non-`nil`.
		if ref.schema != nil {
			t := ref.schema.Type
			if t != nil && *t == "object" {
				typ = "*" + typ
			}
		}

		return typ, nil
	}

	if schema.Type != nil {
		swaggerType := *schema.Type

		if swaggerType == "object" {
			ref := schema.Metadata.(*ref)
			return goType("*") + ref.ToGoType(svcID), nil
		}

		if swaggerType == "string" && schema.Enum != nil {
			ref := schema.Metadata.(*ref)
			return ref.ToGoType(svcID), nil
		}

		if swaggerType == "array" {
			goType, err := swaggerSchemaToGoType(refs, svcID, schema.Items)
			if err != nil {
				return "", fmt.Errorf("couldn't convert swagger Schema to Go type: %w", err)
			}
			return goType, nil
		}

		goType, err := basicTypeSwaggerToGo(swaggerType, optional.OfPtr[string](schema.Format))
		if err != nil {
			return "", fmt.Errorf("couldn't convert basic Swagger type to Go type: %w", err)
		}
		return goType, nil
	}

	return "", fmt.Errorf("schema doesn't define type or reference")
}

func splitParams(
	svcID string,
	name *name,
	refs *refs,
	params []*swagger.Parameter,
) (*paramGroups, error) {
	var optionalBodyType *goType
	path := []*param{}
	query := []*param{}

	for _, rawParam := range params {
		if rawParam.In == "body" {
			if rawParam.Schema == nil {
				return nil, fmt.Errorf("Swagger only supports 'schema's for 'body' parameters ('%s')", rawParam.Name)
			}
			schema := rawParam.Schema

			if schema.Ref == nil {
				optionalBodyType = ptr.Of(goType(name.Pascal() + "Body"))
				continue
			}
			schemaRef := *schema.Ref

			ref, err := refs.LookupSchemaRef(schemaRef)
			if err != nil {
				return nil, fmt.Errorf("couldn't convert Swagger reference to Go type: %w", err)
			}

			optionalBodyType = ptr.Of(ref.ToGoType(svcID))
			continue
		}

		if rawParam.Type == nil {
			return nil, fmt.Errorf("'%s' doesn't have a 'type' property", rawParam.Name)
		}
		paramType := *rawParam.Type

		var goType string
		var funcType string
		if paramType == "integer" {
			// TODO Handle `format`.
			goType = "int"
			funcType = "Int"
		} else if paramType == "string" {
			goType = "string"
			funcType = "String"
		} else if paramType == "number" {
			if rawParam.Format == nil {
				return nil, fmt.Errorf("'number' type must include a 'format'")
			} else if *rawParam.Format != "double" {
				return nil, fmt.Errorf("'%s' is not currently supported as a 'number' format", *rawParam.Format)
			}
			goType = "float64"
			funcType = "Float64"
		} else if paramType == "boolean" {
			goType = "bool"
			funcType = "Bool"
		} else if rawParam.In == "path" {
			return nil, fmt.Errorf("'%s' is not currently supported as a path parameter type", paramType)
		}

		if !rawParam.Required {
			goType = "*" + goType
		}

		name := newNameFromSnake(rawParam.Name)
		if rawParam.In == "path" {
			name = newNameFromCamel(rawParam.Name)
		}

		funcSuffix := funcType
		// TODO Use linting to ensure that all `path` parameters are
		// required.
		if rawParam.In == "query" {
			prefix := "Optional"
			if rawParam.Required {
				prefix = "Required"
			}
			funcSuffix = prefix + funcSuffix
		}

		p := &param{
			Name:       name,
			GoType:     goType,
			IsRequired: rawParam.Required,
			FuncSuffix: snippet(funcSuffix),
		}

		if rawParam.In == "path" {
			path = append(path, p)
		} else if rawParam.In == "query" {
			query = append(query, p)
		} else if rawParam.In == "header" {
			// TODO Add support for `header` parameters.
			continue
		} else {
			return nil, fmt.Errorf("'%s' is not a valid `in` value", rawParam.In)
		}
	}

	sp := &paramGroups{
		OptionalBodyType: optionalBodyType,
		Path:             path,
		Query:            query,
	}
	return sp, nil
}

func newEndpointFromSwagger(
	refs *refs,
	svcID string,
	name *name,
	endpt *swaggerEndpoint,
	params *paramGroups,
	metaType goType,
	context *endpointContext,
) (*endpoint, error) {
	successResp, err := getSuccessResponse(refs, svcID, endpt.Responses)
	if err != nil {
		return nil, fmt.Errorf("couldn't extract success response for endpoint: %w", err)
	}

	metaValues := map[string]snippet{}
	for k, val := range endpt.Server.Meta {
		v, err := renderValue(val)
		if err != nil {
			return nil, fmt.Errorf("couldn't render value: %w", err)
		}
		// TODO Consider having source metadata fields use Go casing.
		name := newNameFromSnake(k).Pascal()
		metaValues[name] = v
	}

	ep := &endpoint{
		Name:   name,
		Method: newNameFromSnake(endpt.method),
		Path:   endpt.path,
		Meta: &endpointMeta{
			GoType: metaType,
			Values: metaValues,
		},
		Context:         context,
		Params:          params,
		SuccessResponse: successResp,
	}
	return ep, nil
}

func renderValue(value any) (snippet, error) {
	switch val := value.(type) {
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	case string:
		return snippet(`"` + val + `"`), nil
	case []any:
		// TODO Allow non-`string`s to be specified in lists.
		vals := "[]string{\n"
		for _, v := range val {
			if _, ok := v.(string); !ok {
				return "", fmt.Errorf("only `string`s are currently supported in lists, got %v (`%T`)", v, v)
			}

			s, err := renderValue(v)
			if err != nil {
				return "", fmt.Errorf("couldn't render value: %w", err)
			}
			vals += string(s) + ",\n"
		}
		return snippet(vals + "}"), nil
	default:
		return "", fmt.Errorf("value '%v' is an unsupported type: %T", value, value)
	}
}

func localContextTypesRawToGoTypes(localContextTypes map[string]string) map[string]goType {
	vs := map[string]goType{}
	for k, v := range localContextTypes {
		vs[k] = goType(v)
	}
	return vs
}
