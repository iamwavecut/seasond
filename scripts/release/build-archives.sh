#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: build-archives.sh <version> [dist-dir]}"
dist_dir="${2:-dist}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ "$dist_dir" != /* ]]; then
  dist_dir="$root/$dist_dir"
fi

mkdir -p "$dist_dir"
rm -f "$dist_dir"/*

build_one() {
  local goos="$1"
  local goarch="$2"
  local archive_os="$3"
  local archive_arch="$4"
  local bin_name="seasond"
  local archive_base="seasond_${version}_${archive_os}_${archive_arch}"
  local work_dir
  work_dir="$(mktemp -d)"
  trap 'rm -rf "$work_dir"' RETURN

  if [[ "$goos" == "windows" ]]; then
    bin_name="seasond.exe"
  fi

  echo "building ${goos}/${goarch}"
  (
    cd "$root"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
      -trimpath \
      -ldflags "-s -w" \
      -o "$work_dir/$bin_name" \
      ./cmd/seasond
  )

  if [[ "$goos" == "windows" ]]; then
    (
      cd "$work_dir"
      zip -q "$dist_dir/${archive_base}.zip" "$bin_name"
    )
  else
    (
      cd "$work_dir"
      tar -czf "$dist_dir/${archive_base}.tar.gz" "$bin_name"
    )
  fi
}

build_one linux amd64 linux x64
build_one linux 386 linux x86
build_one darwin amd64 macos x64
build_one darwin arm64 macos arm64
build_one windows amd64 windows x64
build_one windows 386 windows x86
