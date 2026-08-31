.PHONY: jmeter

# Project name and executable name
PROJECT_NAME := taronja-gateway
BINARY_NAME := tg
ifeq ($(OS),Windows_NT)
	BINARY_NAME := tg.exe
endif

setup:
	@echo "Setting up the project..."
	go mod download
	cd webapp && npm install

# Build target
build: gen
	@echo "Building $(PROJECT_NAME)..."
	cd webapp && npm run build
	CGO_ENABLED=0 go build -tags=purego -o $(BINARY_NAME) .

# Run target
run: build
	@echo "Running $(PROJECT_NAME)..."
	@./$(BINARY_NAME) run --config sample/config.yaml

# Development target with file watching (requires modd)
dev:
	@echo "Starting development mode with file watching..."
	@echo "Using modd from go.mod tools..."
	go run github.com/cortesi/modd/cmd/modd

# Test target
test:
	@echo "Running tests..."
	go test -cover ./...

bench:
	@echo "Running benchmarks..."
	go test -v ./gateway -bench=. -benchtime=2s

# fullbuild mirrors ci.yml's build job end to end: regenerate the OpenAPI
# Go server + TypeScript client and the config docs, then format/vet/build/
# test Go, and build/lint/typecheck the webapp and SDK. Run this — not just
# `make build`/`make test` — before committing anything that touches
# api/taronja-gateway-api.yaml or a config/ doc comment: api/api.gen.go and
# doc/CONFIG.md are committed, generated files, and CI's own "Check
# generated files are up to date" step exists specifically because this is
# easy to forget locally and only fails once it's already in CI.
fullbuild: gen config-docs
	@echo "Formatting and vetting Go code..."
	@UNFORMATTED=$$(gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following files are not gofmt-formatted:"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi
	go vet ./...
	@echo "Building Go..."
	go build -v ./...
	@echo "Building SDK..."
	cd sdk && npm install && npm run build
	@echo "Building webapp..."
	cd webapp && npm run build
	@echo "Linting and type-checking webapp..."
	cd webapp && npm run lint && npx tsc --noEmit
	@echo "Running Go tests..."
	go test -cover ./...
	@echo "fullbuild complete."

# Generate coverage and treemap SVG
cover:
	@echo "Generating coverage report..."
	go test -coverprofile=cover.out ./...
	go tool cover -html=cover.out -o coverage.html

# Release targets
release-check:
	@echo "Checking GoReleaser config..."
	goreleaser check

release-local:
	@echo "Building release locally (no publish)..."
	goreleaser release --snapshot --clean

release-docker:
	@echo "Building Docker image locally..."
	goreleaser release --snapshot --clean --skip-publish

setup-goreleaser:
	@echo "Setting up GoReleaser..."
	@if [ -f ./scripts/setup_goreleaser.sh ]; then \
		bash ./scripts/setup_goreleaser.sh; \
	else \
		echo "setup_goreleaser.sh not found!"; \
		exit 1; \
	fi

# Run JMeter tests
jmeter:
	@echo "Running JMeter..."
	jmeter -t test/test-plan.jmx

# Clean target
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)

# Update dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

gen:
	@echo "Generating OpenAPI code..."
	@go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config api/cfg.yaml api/taronja-gateway-api.yaml
	@echo "Generating TypeScript SDK..."
	@cd webapp && npm install --no-audit --no-fund >/dev/null && npx @hey-api/openapi-ts -i ../api/taronja-gateway-api.yaml -o src/apiclient -c @hey-api/client-fetch

# Generate configuration documentation from Go structs
config-docs:
	@echo "Generating configuration documentation..."
	@go run github.com/princjef/gomarkdoc/cmd/gomarkdoc --output doc/CONFIG.md ./config
	@echo "Configuration documentation generated at doc/CONFIG.md"

install: build
ifeq ($(OS),Windows_NT)
	cp $(BINARY_NAME) ~/bin/$(BINARY_NAME)
else
	cp $(BINARY_NAME) ~/.local/bin/$(BINARY_NAME)
endif

# Default target
.PHONY: all build build-windows run dev test bench fullbuild cover clean fmt tidy
all: build
