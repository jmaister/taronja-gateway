.PHONY: jmeter

# Project name and executable name
PROJECT_NAME := taronja-gateway
BINARY_NAME := tg
ifeq ($(OS),Windows_NT)
	BINARY_NAME := tg.exe
endif

# Build target
build:
	@echo "Building $(PROJECT_NAME)..."
	go build -o $(BINARY_NAME) .

# Run target
run: build
	@echo "Running $(PROJECT_NAME)..."
	@./$(BINARY_NAME) run --config sample/config.yaml

# Test target
test:
	@echo "Running tests..."
	go test -cover ./...

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

# Target to run all k6 tests with a 5-second duration, each test runs only once
k6-test:
	@echo "Running all K6 tests for 5 seconds each, only once..."
	@for file in $(shell find tests -name '*.js' -o -name '*.ts'); do \
		echo "Running $$file..."; \
		k6 run --quiet --duration 5s --iterations 1 $$file; \
	done

# Clean target
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)

# Update dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Default target
.PHONY: all build build-windows run test clean fmt tidy
all: build
