#!/bin/sh
# Install the latest supported Omni release without elevated privileges.
set -eu

repository="toppk/omni"
install_dir="${OMNI_INSTALL_DIR:-$HOME/.local/bin}"
release_url="https://github.com/$repository/releases/latest/download"
target="$install_dir/omni"

case "$(uname -s):$(uname -m)" in
  Linux:x86_64|Linux:amd64) asset="omni_linux_amd64"; checksum_tool="sha256sum" ;;
  Linux:aarch64|Linux:arm64) asset="omni_linux_arm64"; checksum_tool="sha256sum" ;;
  Darwin:x86_64) asset="omni_darwin_amd64"; checksum_tool="shasum" ;;
  Darwin:arm64) asset="omni_darwin_arm64"; checksum_tool="shasum" ;;
  *) echo "omni supports Linux (x86_64, ARM64) and macOS (Intel, Apple Silicon)." >&2; exit 1 ;;
esac

if ! command -v curl >/dev/null 2>&1; then echo "install.sh requires curl." >&2; exit 1; fi
if ! command -v "$checksum_tool" >/dev/null 2>&1; then echo "install.sh requires $checksum_tool." >&2; exit 1; fi

checksum() {
  case "$checksum_tool" in
    sha256sum) sha256sum "$1" | awk '{print $1}' ;;
    shasum) shasum -a 256 "$1" | awk '{print $1}' ;;
  esac
}

temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM
binary="$temporary_dir/$asset"
curl --fail --silent --show-error --location --output "$binary" "$release_url/$asset"
expected=$(curl --fail --silent --show-error --location "$release_url/$asset.sha256" | awk '{print $1}')
actual=$(checksum "$binary")
case "$expected" in *[!0123456789abcdef]*|'') echo "release checksum is invalid." >&2; exit 1 ;; esac
if [ "$expected" != "$actual" ]; then echo "release checksum verification failed." >&2; exit 1; fi

chmod 700 "$binary"
next_version=$("$binary" --version 2>/dev/null || true)
case "$next_version" in
  "omni v"[0-9]*) ;;
  *) echo "downloaded binary does not identify as an Omni release." >&2; exit 1 ;;
esac

previous_version="not installed"
if [ -e "$target" ]; then
  if [ ! -x "$target" ]; then
    echo "refusing to replace non-executable $target." >&2
    exit 1
  fi
  previous_version=$("$target" --version 2>/dev/null || true)
  case "$previous_version" in
    "omni v"[0-9]*) ;;
    *) echo "refusing to replace $target: it does not identify as an Omni release." >&2; exit 1 ;;
  esac
else
  existing_command=$(command -v omni 2>/dev/null || true)
  if [ -n "$existing_command" ] && [ "$existing_command" != "$target" ]; then
    echo "refusing to install: '$existing_command' already provides an unrelated omni command." >&2
    exit 1
  fi
fi

if [ "$previous_version" = "not installed" ]; then
  echo "Installing: not installed → $next_version"
else
  echo "Upgrading: $previous_version → $next_version"
fi

mkdir -p "$install_dir"
temporary_target="$install_dir/.omni-install-$$"
install -m 0755 "$binary" "$temporary_target"
mv -f "$temporary_target" "$target"
echo "Installed $next_version to $target"
case ":$PATH:" in *":$install_dir:"*) ;; *) echo "Add $install_dir to PATH, then run: omni help" ;; esac
