package main

import (
	"fmt"
	"strings"

	"github.com/trustap/rest_api/tools/gen_swagger_server/swagger"
)

func newRefsFromSwaggerDefinitions(
	splitter *refNameSplitter,
	defns map[string]*swagger.Schema,
) *refs {
	return &refs{splitter: splitter, defns: defns}
}

type refs struct {
	splitter *refNameSplitter
	defns    map[string]*swagger.Schema
}

func (rs *refs) LookupSchemaRef(path string) (*schemaRef, error) {
	defnName := strings.TrimPrefix(path, "#/definitions/")
	if defnName == path {
		return nil, fmt.Errorf("unsupported reference: '%s'", path)
	}

	ref, err := rs.splitter.splitRefName(defnName, newNameFromPascal)
	if err != nil {
		return nil, fmt.Errorf("couldn't split referenced definition '%s': %w", defnName, err)
	}

	defn, ok := rs.defns[ref.CanonName()]
	if !ok {
		return nil, fmt.Errorf("couldn't find definition for '%s'", defnName)
	}

	return &schemaRef{ref: ref, schema: defn}, nil
}

type schemaRef struct {
	*ref

	schema *swagger.Schema
}

func newRef(svcID string, name *name) *ref {
	return &ref{SvcID: svcID, Name: name}
}

// A `ref` is a reference to a Swagger type definition. It consists of the type
// name, and the ID of the service in which it's defined.
type ref struct {
	SvcID string
	Name  *name
}

func (r *ref) ToGoType(curSvcID string) goType {
	typ := r.Name.Pascal()
	if curSvcID != r.SvcID {
		typ = r.SvcID + "." + typ
	}
	return goType(typ)
}

func (r *ref) CanonName() string {
	return r.SvcID + "." + r.Name.Pascal()
}

type refNameSplitter struct {
	defaultSvcID string
}

func (s *refNameSplitter) splitRefName(name string, parseName func(string) *name) (*ref, error) {
	parts := strings.Split(name, ".")
	if n := len(parts); n > 2 {
		return nil, fmt.Errorf("name '%s' contains more than 1 '.'", name)
	} else if n > 1 {
		return newRef(parts[0], parseName(parts[1])), nil
	}
	return newRef(s.defaultSvcID, parseName(name)), nil
}
