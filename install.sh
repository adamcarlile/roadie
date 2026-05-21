#!/bin/sh
# roadie installer — downloads the latest release and runs the guided setup.
# Usage: curl -sSL https://raw.githubusercontent.com/adamcarlile/roadie/main/install.sh | sudo sh
set -e

REPO="adamcarlile/roadie"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "roadie: unsupported architecture: $arch" >&2; exit 1 ;;
esac
asset="roadie_${os}_${arch}"
base="https://github.com/${REPO}/releases/latest/download"

# The guided setup prompts on the terminal; under `curl | sh` stdin is the
# piped script, so a real tty must exist for the confirmation step.
if [ ! -c /dev/tty ]; then
  echo "roadie: no terminal available — run install.sh directly, not via a pipe" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${asset}..."
curl -fsSL -o "$tmp/roadie" "${base}/${asset}" ||
  { echo "roadie: failed to download ${asset}" >&2; exit 1; }
curl -fsSL -o "$tmp/checksums.txt" "${base}/checksums.txt" ||
  { echo "roadie: failed to download checksums.txt" >&2; exit 1; }

# Verify the checksum. grep is kept out of the pipeline so a missing entry is
# caught explicitly — set -e does not propagate a mid-pipeline failure.
echo "Verifying checksum..."
sum=$(grep " ${asset}\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if [ -z "$sum" ]; then
  echo "roadie: no checksum listed for ${asset}" >&2
  exit 1
fi
echo "${sum}  ${tmp}/roadie" | sha256sum -c - >/dev/null ||
  { echo "roadie: checksum verification failed for ${asset}" >&2; exit 1; }

chmod +x "$tmp/roadie"
echo "Starting guided setup..."
# Read the confirmation prompt from the real terminal, since stdin here is the
# piped install script, not the tty. Do not exec, so the trap cleans up $tmp.
"$tmp/roadie" install < /dev/tty
