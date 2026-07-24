#!/bin/sh
# Install the latest Linux x86_64 Omni release without elevated privileges.
set -eu

repository="toppk/omni"
asset="omni_linux_amd64"
install_dir="${OMNI_INSTALL_DIR:-$HOME/.local/bin}"
release_url="https://github.com/$repository/releases/latest/download"

case "$(uname -s):$(uname -m)" in
  Linux:x86_64|Linux:amd64) ;;
  *) echo "omni currently publishes Linux x86_64 binaries only." >&2; exit 1 ;;
esac

if ! command -v curl >/dev/null 2>&1; then echo "install.sh requires curl." >&2; exit 1; fi
if ! command -v sha256sum >/dev/null 2>&1; then echo "install.sh requires sha256sum." >&2; exit 1; fi

temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM
binary="$temporary_dir/$asset"
curl --fail --silent --show-error --location --output "$binary" "$release_url/$asset"
expected=$(curl --fail --silent --show-error --location "$release_url/$asset.sha256" | awk '{print $1}')
actual=$(sha256sum "$binary" | awk '{print $1}')
case "$expected" in *[!0123456789abcdef]*|'') echo "release checksum is invalid." >&2; exit 1 ;; esac
if [ "$expected" != "$actual" ]; then echo "release checksum verification failed." >&2; exit 1; fi
mkdir -p "$install_dir"
install -m 0755 "$binary" "$install_dir/omni"
echo "Installed omni to $install_dir/omni"
case ":$PATH:" in *":$install_dir:"*) ;; *) echo "Add $install_dir to PATH, then run: omni help" ;; esac
