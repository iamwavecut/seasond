#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST="$(mktemp -d)"
trap 'rm -rf "$DIST"' EXIT

"$ROOT/scripts/release/build-archives.sh" v0.0.0-test "$DIST" >/dev/null

tar -tzf "$DIST/seasond_v0.0.0-test_linux_x64.tar.gz" | grep -qx 'seasond'
tar -tzf "$DIST/seasond_v0.0.0-test_macos_arm64.tar.gz" | grep -qx 'seasond'
unzip -Z1 "$DIST/seasond_v0.0.0-test_windows_x64.zip" | grep -qx 'seasond.exe'

old_binary="$(printf '%s%s' mod guard)"
if tar -tzf "$DIST/seasond_v0.0.0-test_linux_x64.tar.gz" | grep -qx "$old_binary"; then
  echo "archive unexpectedly contains previous CLI binary name" >&2
  exit 1
fi

echo "ok"
