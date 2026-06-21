#!/bin/sh
# agentop installer for macOS / Linux.
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/thesatellite-ai/agentop/main/install.sh | sh
#
# Detects OS + arch, downloads the latest released binary from GitHub Releases,
# and installs it to /usr/local/bin.

set -e

REPO="thesatellite-ai/agentop"
BINARY="agentop"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS" && exit 1 ;;
esac

ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"
# Use GitHub's /releases/latest/download/ redirect — resolves the newest
# non-prerelease asset WITHOUT calling the rate-limited GitHub API.
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading latest ${BINARY} for ${OS}/${ARCH}..."
tmpdir=$(mktemp -d)
if ! curl -fsSL "$URL" | tar xz -C "$tmpdir" 2>/dev/null; then
  echo "Error: could not download ${ASSET} from" >&2
  echo "  ${URL}" >&2
  echo "Check that a release exists: https://github.com/${REPO}/releases" >&2
  exit 1
fi

if [ ! -f "$tmpdir/$BINARY" ]; then
  echo "Error: ${BINARY} binary not found in archive." && exit 1
fi

echo "Installing to ${INSTALL_DIR}/${BINARY}..."
if [ -w "$INSTALL_DIR" ]; then
  mv "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY"
else
  sudo mv "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY"
fi
chmod +x "$INSTALL_DIR/$BINARY"
rm -rf "$tmpdir"

echo ""
echo "Installed. Try:"
echo "  ${BINARY}            # interactive TUI"
echo "  ${BINARY} list       # one-shot table"
echo "  ${BINARY} reap --idle 2"
echo "Verify: ${BINARY} version"
