.PHONY: jmeter

# Project name and executable name
PROJECT_NAME := taronja-gateway
BINARY_NAME := tg # This is the name of the executable for Linux
WINDOWS_BINARY_NAME := tg.exe # This is the name of the executable for Windows

# Source files
SRC_FILES := $(shell find . -name '*.go' -type f)

# Build target
build:
	@echo "Building $(PROJECT_NAME)..."
	go build -o $(BINARY_NAME) .

# Build for Windows
build-windows:
	@echo "Building $(PROJECT_NAME) for Windows..."
	GOOS=windows GOARCH=amd64 go build -o $(WINDOWS_BINARY_NAME) .

# Run target
run: build
	@echo "Running $(PROJECT_NAME)..."
	./$(BINARY_NAME) sample/config.yaml

# Test target
test:
	@echo "Running tests..."
	go test ./...

# Run JMeter tests
jmeter:
	@echo "Running JMeter..."
	jmeter -t test/test-plan.jmx


# Clean target
clean:
	@echo "Cleaning up..."
	go clean
	rm -f $(BINARY_NAME) $(WINDOWS_BINARY_NAME) # Remove both executables

# Update dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Default target
.PHONY: all build build-windows run test clean fmt tidy
all: build
