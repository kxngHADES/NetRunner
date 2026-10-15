#!/usr/bin/env sh
# Install the latest NetRunner release (Linux/macOS).
#   curl -fsSL https://raw.githubusercontent.com/kxngHADES/NetRunner/main/scripts/install.sh | sh
# Optional: VERSION=v0.1.0  INSTALL_DIR=$HOME/.local/bin
set -eu

REPO="kxngHADES/NetRunner"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in linux|darwin) ;; *) echo "unsupported OS: $os" >&2; exit 1 ;; esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ -z "${VERSION:-}" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
fi
[ -n "$VERSION" ] || { echo "could not determine latest version (no releases yet?)" >&2; exit 1; }

ver="${VERSION#v}"
archive="netrunner_${ver}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$VERSION"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading NetRunner $VERSION ($os/$arch)..."
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
else actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}'); fi
[ -n "$expected" ] && [ "$expected" = "$actual" ] || { echo "checksum verification failed" >&2; exit 1; }

tar -xzf "$tmp/$archive" -C "$tmp" netrunner
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/netrunner" "$INSTALL_DIR/netrunner"

echo "Installed to $INSTALL_DIR/netrunner"
case ":$PATH:" in *":$INSTALL_DIR:"*) ;; *) echo "Add $INSTALL_DIR to your PATH to run 'netrunner'." ;; esac
