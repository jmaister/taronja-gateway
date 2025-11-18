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
	npx --yes @hey-api/openapi-ts -i ./api/taronja-gateway-api.yaml -o webapp/src/apiclient -c @hey-api/client-fetch

# Generate configuration documentation from Go structs
config-docs:
	@echo "Generating configuration documentation..."
	@gomarkdoc --output doc/CONFIG.md ./config
	@echo "Configuration documentation generated at doc/CONFIG.md"

install: build
ifeq ($(OS),Windows_NT)
	cp $(BINARY_NAME) ~/bin/$(BINARY_NAME)
else
	cp $(BINARY_NAME) ~/.local/bin/$(BINARY_NAME)
endif

# Default target
.PHONY: all build build-windows run dev test bench cover clean fmt tidy
all: build
