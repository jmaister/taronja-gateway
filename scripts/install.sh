#!/usr/bin/env bash
# Linux/macOS installer for taronja-gateway (tg)
# Downloads the latest GoReleaser artifact from GitHub and installs tg to $HOME/.local/bin

set -e
REPO="jmaister/taronja-gateway"
TG_BIN="tg"
INSTALL_DIR="$HOME/.local/bin"
API_URL="https://api.github.com/repos/$REPO/releases/latest"

mkdir -p "$INSTALL_DIR"

# Detect OS and ARCH
OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
    Linux)
        PLATFORM="Linux" ;;
    Darwin)
        PLATFORM="Darwin" ;;
    *)
        echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
    x86_64|amd64)
        ARCHSTR="x86_64" ;;
    arm64|aarch64)
        ARCHSTR="arm64" ;;
    armv7*)
        ARCHSTR="armv7" ;;
    *)
        echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

TAR_NAME="${PLATFORM}_${ARCHSTR}.tar.gz"
TAR_URL=$(curl -s "$API_URL" | grep browser_download_url | grep "$TAR_NAME" | cut -d '"' -f 4)
if [ -z "$TAR_URL" ]; then
    echo "ERROR: Could not find a $PLATFORM $ARCHSTR GoReleaser artifact download URL in the latest release."
    exit 1
fi

echo "Downloading from: $TAR_URL"
curl -L "$TAR_URL" -o tg_dist.tar.gz

# Extract the tar.gz (GoReleaser puts binary in a subfolder)
EXTRACT_DIR=$(mktemp -d)
tar -xzf tg_dist.tar.gz -C "$EXTRACT_DIR"

# Find the extracted folder (should match taronja-gateway*)
EXTRACTED_DIR=$(find "$EXTRACT_DIR" -type d -name 'taronja-gateway*' | head -n1)

# Move tg from extracted folder to install dir
if [ -f "$EXTRACTED_DIR/$TG_BIN" ]; then
    mv "$EXTRACTED_DIR/$TG_BIN" "$INSTALL_DIR/tg"
    chmod +x "$INSTALL_DIR/tg"
elif [ -f "$EXTRACT_DIR/$TG_BIN" ]; then
    mv "$EXTRACT_DIR/$TG_BIN" "$INSTALL_DIR/tg"
    chmod +x "$INSTALL_DIR/tg"
else
    echo "ERROR: tg not found after extraction. Please check the contents of $EXTRACT_DIR."
    exit 1
fi

# Clean up
rm -rf tg_dist.tar.gz "$EXTRACT_DIR"

echo
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo "Add $INSTALL_DIR to your PATH to use 'tg' from anywhere."
    echo "For bash/zsh, you can run:"
    echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.profile && source ~/.profile"
else
    echo "'tg' is installed to $INSTALL_DIR, which is already in your PATH."
fi

echo "Done."
