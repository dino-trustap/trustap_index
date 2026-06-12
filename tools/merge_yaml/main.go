// `$0 <yaml-file+>` merges the root objects defined in the `yaml-file`s and
// renders the resulting object as YAML.

package main

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	argv := os.Args
	if len(argv) < 2 {
		return fmt.Errorf("usage: %s <yaml-file+>", argv[0])
	}

	root := map[string]any{}
	for _, path := range argv[1:] {
		rawPartial, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("couldn't read YAML file '%s': %w", path, err)
		}

		partial := map[string]any{}
		err = yaml.Unmarshal(rawPartial, partial)
		if err != nil {
			return fmt.Errorf("couldn't parse YAML file '%s': %w", path, err)
		}

		root, err = mergeMaps(root, partial)
		if err != nil {
			return fmt.Errorf("couldn't merge YAML file '%s' with other files: %w", path, err)
		}
	}

	bs, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("couldn't render YAML: %w", err)
	}

	fmt.Println(string(bs))

	return nil
}

func mergeMaps(a, b map[string]any) (map[string]any, error) {
	c := map[string]any{}

	for k, v := range a {
		c[k] = v
	}

	for k, v := range b {
		if _, exists := c[k]; exists {
			var err error
			c[k], err = merge(c[k], v)
			if err != nil {
				var mergeErr *mergeError
				if ok := errors.As(err, &mergeErr); ok {
					path := fmt.Sprintf("%v", k)
					if mergeErr.path != "" {
						path += "." + mergeErr.path
					}
					return nil, newMergeError(path, mergeErr.err)
				}
				return nil, fmt.Errorf("couldn't merge maps with key '%v': %w", k, err)
			}
		} else {
			c[k] = v
		}
	}

	return c, nil
}

func merge(a, b any) (any, error) {
	if aMap, ok := a.(map[string]any); ok {
		if bMap, ok := b.(map[string]any); ok {
			c, err := mergeMaps(aMap, bMap)
			if err != nil {
				return nil, fmt.Errorf("couldn't merge maps: %w", err)
			}
			return c, nil
		}
	}

	e := fmt.Errorf("couldn't merge unsupported type pair (type '%T' and '%T')", a, b)
	return nil, newMergeError("", e)
}

func newMergeError(path string, err error) error {
	return &mergeError{path: path, err: err}
}

type mergeError struct {
	path string
	err  error
}

func (e *mergeError) Error() string {
	return fmt.Sprintf("couldn't merge nodes at '%s': %v", e.path, e.err)
}
