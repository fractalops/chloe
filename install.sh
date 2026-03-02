#!/bin/bash
set -euo pipefail

REPO="fractalops/chloe"
BINARY="chloe"

OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
  Linux)  OS="Linux" ;;
  Darwin) OS="Darwin" ;;
  *)      echo "Unsupported OS: $OS (chloe supports Linux and macOS only)"; exit 1 ;;
esac

case "$ARCH" in
  x86_64)  ARCH="x86_64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

TAG=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release"
  exit 1
fi

ASSET="${BINARY}_${OS}_${ARCH}.zip"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

echo "Downloading ${BINARY} ${TAG} for ${OS}/${ARCH}..."

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -sL "${BASE_URL}/${ASSET}" -o "${TMP}/${ASSET}"
curl -sL "${BASE_URL}/checksums.txt" -o "${TMP}/checksums.txt" 2>/dev/null || true

# Verify checksum if checksums.txt was available
if [ -f "${TMP}/checksums.txt" ] && [ -s "${TMP}/checksums.txt" ]; then
  EXPECTED=$(grep "${ASSET}" "${TMP}/checksums.txt" | awk '{print $1}')
  if [ -n "$EXPECTED" ]; then
    if command -v sha256sum &>/dev/null; then
      ACTUAL=$(sha256sum "${TMP}/${ASSET}" | awk '{print $1}')
    else
      ACTUAL=$(shasum -a 256 "${TMP}/${ASSET}" | awk '{print $1}')
    fi
    if [ "$EXPECTED" != "$ACTUAL" ]; then
      echo "Checksum verification failed!"
      echo "  Expected: $EXPECTED"
      echo "  Actual:   $ACTUAL"
      exit 1
    fi
    echo "Checksum verified."
  fi
fi

unzip -q "${TMP}/${ASSET}" -d "$TMP"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMP}/${BINARY}" "$INSTALL_DIR/${BINARY}"
else
  mv "${TMP}/${BINARY}" "$INSTALL_DIR/${BINARY}"
fi

chmod +x "$INSTALL_DIR/${BINARY}"
echo "${BINARY} ${TAG} installed to ${INSTALL_DIR}/${BINARY}"
