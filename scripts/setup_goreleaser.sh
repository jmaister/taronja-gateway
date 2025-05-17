#!/bin/bash

# This script helps set up GoReleaser for development use

set -e

# Detect operating system
OS="$(uname -s)"
ARCH="$(uname -m)"

echo "Setting up GoReleaser for $OS $ARCH..."

# macOS
if [ "$OS" = "Darwin" ]; then
  if command -v brew &> /dev/null; then
    echo "Installing GoReleaser using Homebrew..."
    brew install goreleaser
  else
    echo "Homebrew not found. Please install it first: https://brew.sh/"
    exit 1
  fi
# Linux
elif [ "$OS" = "Linux" ]; then
  # Check for Ubuntu/Debian
  if command -v apt-get &> /dev/null; then
    echo "Installing GoReleaser on Debian/Ubuntu..."
    echo "deb [trusted=yes] https://repo.goreleaser.com/apt/ /" | sudo tee /etc/apt/sources.list.d/goreleaser.list
    sudo apt update
    sudo apt install goreleaser
  # Check for RHEL/CentOS/Fedora
  elif command -v dnf &> /dev/null || command -v yum &> /dev/null; then
    echo "Installing GoReleaser on RHEL/CentOS/Fedora..."
    if command -v dnf &> /dev/null; then
      sudo dnf install -y https://repo.goreleaser.com/yum/goreleaser-repo-latest.rpm
      sudo dnf install -y goreleaser
    else
      sudo yum install -y https://repo.goreleaser.com/yum/goreleaser-repo-latest.rpm
      sudo yum install -y goreleaser
    fi
  else
    echo "Could not detect package manager. Installing using Go..."
    go install github.com/goreleaser/goreleaser@latest
  fi
# Windows (assumes running in Git Bash, WSL, or similar)
elif [[ "$OS" == MINGW* ]] || [[ "$OS" == MSYS* ]] || [[ "$OS" == CYGWIN* ]]; then
  echo "Installing GoReleaser on Windows..."
  if command -v go &> /dev/null; then
    go install github.com/goreleaser/goreleaser@latest
  else
    echo "Go not found. Please install Go first: https://golang.org/dl/"
    exit 1
  fi
# Unknown OS
else
  echo "Unsupported operating system: $OS"
  echo "Installing using Go if available..."
  if command -v go &> /dev/null; then
    go install github.com/goreleaser/goreleaser@latest
  else
    echo "Go not found. Please install Go first: https://golang.org/dl/"
    exit 1
  fi
fi

echo "GoReleaser setup complete!"
echo "Try running: goreleaser --version"
