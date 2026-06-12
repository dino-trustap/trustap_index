package main

import (
	"fmt"
	"strconv"

	"github.com/trustap/rest_api/tools/gen_swagger_server/swagger"
)

func setInlineDefnNamesOnEndpts(splitter *refNameSplitter, endpts []*swaggerEndpoint) error {
	for _, endpt := range endpts {
		ref, err := splitter.splitRefName(endpt.OperationID, newNameFromCamel)
		if err != nil {
			return fmt.Errorf("couldn't split referenced operation ID '%s': %w", endpt.OperationID, err)
		}

		for _, param := range endpt.Parameters {
			if param.In != "body" || param.Schema == nil {
				continue
			}
			name := ref.Name.Pascal() + "Body"

			if param.Schema == nil {
				return fmt.Errorf("parameter '%s' doesn't have a schema", name)
			}

			err := addInlineDefnNamesToSchema(param.Schema, ref.SvcID, name)
			if err != nil {
				return fmt.Errorf("couldn't set inline definition name on '%s' body: %w", endpt.OperationID, err)
			}
		}

		for statusCode, resp := range endpt.Responses {
			if resp.Schema == nil {
				continue
			}
			name := ref.Name.Pascal() + "Resp" + strconv.Itoa(statusCode)

			if resp.Schema == nil {
				return fmt.Errorf("response '%s' doesn't have a schema", name)
			}

			err := addInlineDefnNamesToSchema(resp.Schema, ref.SvcID, name)
			if err != nil {
				msg := "couldn't set inline definition name on '%s' %d response: %w"
				return fmt.Errorf(msg, endpt.OperationID, statusCode, err)
			}
		}
	}

	return nil
}

func addInlineDefnNamesToSchema(schema *swagger.Schema, svcID, name string) error {
	if schema.Ref != nil {
		return nil
	}

	if schema.Type != nil {
		typ := *schema.Type

		if typ == "object" {
			schema.Metadata = newRef(svcID, newNameFromPascal(name))

			for propName, prop := range schema.Properties {
				suf := newNameFromSnake(propName).Pascal()

				if prop == nil {
					return fmt.Errorf("property '%s' doesn't have a schema", name+suf)
				}

				err := addInlineDefnNamesToSchema(prop, svcID, name+suf)
				if err != nil {
					return fmt.Errorf("couldn't set inline definition name on property '%s' of '%s': %w", propName, name, err)
				}
			}
		}

		if typ == "string" && schema.Enum != nil {
			schema.Metadata = newRef(svcID, newNameFromPascal(name))
		}

		if typ == "array" {
			if schema.Items == nil {
				return fmt.Errorf("array items '%s' don't have a schema", name+"Item")
			}

			err := addInlineDefnNamesToSchema(schema.Items, svcID, name+"Item")
			if err != nil {
				return fmt.Errorf("couldn't set inline definition name on items of '%s': %w", name, err)
			}
		}

		return nil
	}

	return fmt.Errorf("schema '%s' doesn't define type or reference", name)
}

func setInlineDefnNamesOnDefinitions(splitter *refNameSplitter, defns map[string]*swagger.Schema) error {
	for defnName, defn := range defns {
		ref, err := splitter.splitRefName(defnName, newNameFromPascal)
		if err != nil {
			return fmt.Errorf("couldn't split referenced definition '%s': %w", defnName, err)
		}

		if defn == nil {
			return fmt.Errorf("definition '%s' doesn't have a schema", defnName)
		}

		err = addInlineDefnNamesToSchema(defn, ref.SvcID, ref.Name.Pascal())
		if err != nil {
			return fmt.Errorf("couldn't set inline definition name on '%s': %w", defnName, err)
		}
	}

	return nil
}

func extractInlineDefinitionsFromSwaggerPaths(endpts []*swaggerEndpoint) map[string]*swagger.Schema {
	defns := map[string]*swagger.Schema{}

	for _, endpt := range endpts {
		for _, param := range endpt.Parameters {
			if param.In != "body" || param.Schema == nil {
				continue
			}

			for name, defn := range extractInlineDefinitionsFromSwaggerSchema(param.Schema) {
				defns[name] = defn
			}
		}

		for _, resp := range endpt.Responses {
			if resp.Schema == nil {
				continue
			}

			for name, defn := range extractInlineDefinitionsFromSwaggerSchema(resp.Schema) {
				defns[name] = defn
			}
		}
	}

	return defns
}

func extractInlineDefinitionsFromSwaggerSchema(schema *swagger.Schema) map[string]*swagger.Schema {
	defns := map[string]*swagger.Schema{}

	if schema.Type == nil {
		// TODO Allow traversing `$ref`s in order to find more
		// definitions.
		return defns
	}
	schemaType := string(*schema.Type)

	if schemaType == "object" {
		ref := schema.Metadata.(*ref)
		defns[ref.CanonName()] = schema

		for _, prop := range schema.Properties {
			for name, defn := range extractInlineDefinitionsFromSwaggerSchema(prop) {
				defns[name] = defn
			}
		}
	}

	if schemaType == "string" && schema.Enum != nil {
		ref := schema.Metadata.(*ref)
		defns[ref.CanonName()] = schema
	}

	if schemaType == "array" {
		for name, defn := range extractInlineDefinitionsFromSwaggerSchema(schema.Items) {
			defns[name] = defn
		}
	}

	return defns
}

func extractInlineDefinitionsFromSwaggerDefinitions(defns map[string]*swagger.Schema) map[string]*swagger.Schema {
	extractedDefns := map[string]*swagger.Schema{}

	for _, defn := range defns {
		for name, defn := range extractInlineDefinitionsFromSwaggerSchema(defn) {
			extractedDefns[name] = defn
		}
	}

	return extractedDefns
}
