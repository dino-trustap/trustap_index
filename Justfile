project := 'trustap_index'
img := 'trustap' / project
tgt_dir := 'target'
tgt_artfs_dir := tgt_dir / 'artfs'
tgt := tgt_artfs_dir / 'api'
tgt_tmp_dir := tgt_dir / 'tmp'
tgt_gen_dir := tgt_dir / 'gen'
tgt_img_ctx_dir := tgt_tmp_dir / 'img_ctx'

src_dirs := 'cmd internal pkg tools'

# List available recipes.
default:
    just --list

# Build the API binary.
build:
    make api

# Run the API server locally (without config).
run addr='0.0.0.0:8080': build
    '{{tgt}}' '{{addr}}'

# Run the API server locally with config.
run-with-config conf='configs/api.yaml' addr='0.0.0.0:8080': build
    '{{tgt}}' '{{conf}}' '{{addr}}'

# Generate Go code from Swagger specs.
gen_code:
    make '{{tgt_gen_dir}}/swagger_server/endpoint.go'

# Clean build artifacts.
clean:
    make clean

# Build Docker image.
build_img: build
    mkdir -p '{{tgt_img_ctx_dir}}'
    cp '{{tgt}}' '{{tgt_img_ctx_dir}}/api'
    cp Dockerfile '{{tgt_img_ctx_dir}}/Dockerfile'
    docker build \
        --tag='{{img}}:latest' \
        '{{tgt_img_ctx_dir}}'

# Run the API from Docker container.
run_cont addr='0.0.0.0:8080': build_img
    docker run \
        --rm \
        --interactive \
        --tty \
        --publish '{{addr}}:80' \
        '{{img}}:latest'

# Run style checks for Go files.
check_style:
    @# Check gofumpt, goimports, and gofmt formatting
    ! (gofumpt -d {{src_dirs}} | grep '')
    ! (goimports -d {{src_dirs}} | grep '')
    ! (gofmt -s -d {{src_dirs}} | grep '')

# Format Go code.
fmt:
    gofumpt -w {{src_dirs}}
    goimports -w {{src_dirs}}
    gofmt -s -w {{src_dirs}}

# Run all checks.
check: check_style build
    @echo "All checks passed!"

