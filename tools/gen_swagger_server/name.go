package main

import (
	"strings"
	"unicode"
)

type name struct {
	parts []string
}

var initialisms = map[string]string{
	"id":  "ID",
	"ids": "IDs",
	"api": "API",
}

// TODO Update constructors to throw an error if the name contains invalid
// characters for the specified format.

func newNameFromPascal(s string) *name {
	parts := []string{}
	j := 0
	for i, c := range s {
		if i == 0 {
			continue
		}

		if unicode.IsUpper(c) {
			parts = append(parts, s[j:i])
			j = i
		}
	}
	parts = append(parts, s[j:])
	return &name{parts: parts}
}

func newNameFromCamel(s string) *name {
	parts := []string{}
	j := 0
	for i, c := range s {
		if unicode.IsUpper(c) {
			parts = append(parts, s[j:i])
			j = i
		}
	}
	parts = append(parts, s[j:])
	return &name{parts: parts}
}

func newNameFromSnake(s string) *name {
	return &name{parts: strings.Split(s, "_")}
}

func (n *name) Pascal() string {
	parts := make([]string, len(n.parts))
	for i, part := range n.parts {
		parts[i] = applyTitleCase(part)
	}
	return strings.Join(parts, "")
}

func applyTitleCase(s string) string {
	lower := strings.ToLower(s)
	if initialism, ok := initialisms[lower]; ok {
		return initialism
	}
	return strings.ToUpper(lower[:1]) + strings.ToLower(lower[1:])
}

func (n *name) Camel() string {
	parts := make([]string, len(n.parts))
	for i, part := range n.parts[1:] {
		parts[i] = applyTitleCase(part)
	}
	return strings.ToLower(n.parts[0]) + strings.Join(parts, "")
}

func (n *name) Snake() string {
	parts := make([]string, len(n.parts))
	for i, part := range n.parts {
		parts[i] = strings.ToLower(part)
	}
	return strings.Join(parts, "_")
}
