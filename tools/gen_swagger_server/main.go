package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/trustap/rest_api/tools/gen_swagger_server/swagger"
	"gopkg.in/yaml.v3"
)

func main() {
	err := genSwaggerServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "couldn't generate Swagger server: %v\n", err)
		os.Exit(1)
	}
}

func genSwaggerServer() error {
	argv := os.Args
	if len(argv) != 4 {
		return fmt.Errorf("usage: %s <swagger-yaml> <config-yaml> <out-dir>", argv[0])
	}
	swaggerYamlPath := argv[1]
	configYamlPath := argv[2]
	outDir := argv[3]

	configYaml, err := os.ReadFile(configYamlPath)
	if err != nil {
		return fmt.Errorf("couldn't read config file`: %w", err)
	}

	config := &config{}
	err = yaml.Unmarshal(configYaml, config)
	if err != nil {
		return fmt.Errorf("couldn't parse YAML config: %w", err)
	}

	swaggerYaml, err := os.ReadFile(swaggerYamlPath)
	if err != nil {
		return fmt.Errorf("couldn't read config file`: %w", err)
	}

	swaggerSpec := &swagger.Spec{}
	err = yaml.Unmarshal(swaggerYaml, swaggerSpec)
	if err != nil {
		return fmt.Errorf("couldn't parse swagger YAML: %w", err)
	}

	templs, err := parseTemplateGlobs([]string{
		"assets/swagger_codegen_templates/*.gotempl",
		"assets/swagger_codegen_templates/partials/*.gotempl",
	})
	if err != nil {
		return fmt.Errorf("couldn't parse templates: %w", err)
	}

	files, err := renderFiles(templs, config, swaggerSpec)
	if err != nil {
		return fmt.Errorf("couldn't render files: %w", err)
	}

	for subPath, contents := range files {
		outFile := filepath.Join(outDir, subPath)

		// TODO Handle error.
		_ = writeFileAtPath(outFile, string(contents), filePerm)
	}

	return nil
}

const filePerm = 0o600

type config struct {
	GoPackage        *packageConfig          `yaml:"go_package"`
	DefaultService   string                  `yaml:"default_service"`
	IncludeEndpoints map[string][]string     `yaml:"include_endpoints"`
	EndpointMetadata *endpointMetadataConfig `yaml:"endpoint_metadata"`
	ContextTypes     *contextTypesConfig     `yaml:"context_types"`
}

type packageConfig struct {
	Path string `yaml:"path"`
	Name string `yaml:"name"`
}

type endpointMetadataConfig struct {
	Import string `yaml:"import"`
	Type   string `yaml:"type"`
}

type contextTypesConfig struct {
	Import string            `yaml:"import"`
	Global string            `yaml:"global"`
	Local  map[string]string `yaml:"local"`
}

func parseTemplateGlobs(patterns []string) (*template.Template, error) {
	templs := &template.Template{}
	for _, pattern := range patterns {
		_, err := templs.ParseGlob(pattern)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse template files matching '%s': %w", pattern, err)
		}
	}
	return templs, nil
}

func writeFileAtPath(path, data string, filePerm os.FileMode) error {
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err != nil {
		return fmt.Errorf("couldn't create parent directories: %w", err)
	}

	err = os.WriteFile(path, []byte(data), filePerm)
	if err != nil {
		return fmt.Errorf("couldn't write file: %w", err)
	}

	return nil
}
