tgt_dir:=target
tgt_artfs_dir:=$(tgt_dir)/artfs
tgt_gen_dir:=$(tgt_dir)/gen
tgt_tmp_dir:=$(tgt_dir)/tmp
pkg:=github.com/trustap/trustap_index

# Source directories
src_dirs:= \
	cmd \
	internal \
	pkg \
	tools

# Swagger specifications
swagger_specs:=$(wildcard api/swagger/*.yaml)
swagger_codegen_templs:= \
	$(wildcard assets/swagger_codegen_templates/*.gotempl) \
	$(wildcard assets/swagger_codegen_templates/partials/*.gotempl)

deps:= \
	$(shell find $(src_dirs) -name '*.go' -type f 2>/dev/null) \
	$(shell find $(tgt_artfs_dir) -name '*.go' -type f 2>/dev/null) \
	$(shell find frontend/dist frontend/embed.go -type f 2>/dev/null) \
	$(tgt_gen_dir)/swagger_server/endpoint.go

# Build all artefacts.
.PHONY: artfs
artfs: api

# Build the API.
.PHONY: api
api: $(tgt_artfs_dir)/api

$(tgt_artfs_dir)/api: $(deps) | $(tgt_artfs_dir)
	@# We disable cgo when building so that these executables can be run in
	@# environments outside those that they were built in.
	( \
		cd cmd/api \
			&& go get -v \
			&& CGO_ENABLED=0 go build \
				-o '../../$@' \
	)

# Generate swagger server code
$(tgt_gen_dir)/swagger_server/endpoint.go: \
		$(tgt_gen_dir)/swagger_api/api.yaml \
		configs/build/swagger_server.yaml \
		$(swagger_codegen_templs) \
		$(tgt_gen_dir)/gen_swagger_server \
		| $(tgt_gen_dir)
	rm -rf $(tgt_gen_dir)/swagger_server
	$(tgt_gen_dir)/gen_swagger_server \
		'$<' \
		'configs/build/swagger_server.yaml' \
		'$(tgt_gen_dir)/swagger_server'
	gofumpt -w '$(tgt_gen_dir)/swagger_server' || true
	goimports -w '$(tgt_gen_dir)/swagger_server' || true

# Build the swagger code generator
$(tgt_gen_dir)/gen_swagger_server: \
		$(wildcard assets/swagger_codegen_templates/*.gotempl) \
		$(wildcard assets/swagger_codegen_templates/partials/*.gotempl) \
		$(wildcard tools/gen_swagger_server/*.go) \
		$(wildcard tools/gen_swagger_server/swagger/*.go) \
		$(wildcard tools/gen_swagger_server/swagger_rest/*.go) \
		| $(tgt_gen_dir)
	( \
		cd tools/gen_swagger_server \
			&& go get -v \
			&& go build -o '../../$@' \
	)

# Merge swagger YAML files
$(tgt_gen_dir)/swagger_api/api.yaml: \
		$(swagger_specs) \
		| $(tgt_gen_dir)/swagger_api
	go run tools/merge_yaml/main.go \
		$^ \
		> '$@_'
	mv '$@_' '$@'

# Directory creation
$(tgt_artfs_dir): | $(tgt_dir)
	mkdir '$@'

$(tgt_gen_dir): | $(tgt_dir)
	mkdir '$@'

$(tgt_gen_dir)/swagger_api: | $(tgt_gen_dir)
	mkdir '$@'

$(tgt_gen_dir)/swagger_server: | $(tgt_gen_dir)
	mkdir '$@'

$(tgt_tmp_dir): | $(tgt_dir)
	mkdir '$@'

$(tgt_dir):
	mkdir '$@'

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(tgt_dir)

# Run the API server (without config)
.PHONY: run
run: api
	./$(tgt_artfs_dir)/api :8080

# Run the API server with config
.PHONY: run-with-config
run-with-config: api
	./$(tgt_artfs_dir)/api configs/api.yaml :8080

