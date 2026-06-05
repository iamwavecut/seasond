#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/release/next-version.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

make_repo() {
  local dir
  dir="$(mktemp -d)"
  git -C "$dir" init -b main >/dev/null
  git -C "$dir" config user.name "Release Test"
  git -C "$dir" config user.email "release-test@example.com"
  echo initial >"$dir/file.txt"
  git -C "$dir" add file.txt
  git -C "$dir" commit -m "chore: initial" >/dev/null
  git -C "$dir" tag v1.2.3
  printf '%s\n' "$dir"
}

commit_msg() {
  local dir="$1"
  local subject="$2"
  local body="${3:-}"
  printf '%s\n' "$subject" >>"$dir/file.txt"
  git -C "$dir" add file.txt
  if [[ -n "$body" ]]; then
    git -C "$dir" commit -m "$subject" -m "$body" >/dev/null
  else
    git -C "$dir" commit -m "$subject" >/dev/null
  fi
}

assert_version() {
  local name="$1"
  local expected_version="$2"
  local expected_type="$3"
  local dir
  dir="$(make_repo)"
  shift 3
  "$@" "$dir"

  local output version release_type
  output="$(cd "$dir" && "$SCRIPT")"
  version="$(printf '%s\n' "$output" | awk -F= '$1 == "version" {print $2}')"
  release_type="$(printf '%s\n' "$output" | awk -F= '$1 == "release_type" {print $2}')"

  [[ "$version" == "$expected_version" ]] || fail "$name: version=$version want $expected_version"
  [[ "$release_type" == "$expected_type" ]] || fail "$name: release_type=$release_type want $expected_type"
}

patch_case() {
  commit_msg "$1" "fix: avoid stale index cursor"
}

minor_case() {
  commit_msg "$1" "fix: keep readyz conservative"
  commit_msg "$1" "feat: add release archives"
}

major_bang_case() {
  commit_msg "$1" "feat!: rename command"
}

major_footer_case() {
  commit_msg "$1" "feat: change config layout" "BREAKING CHANGE: DB_PATH is now required."
}

none_case() {
  commit_msg "$1" "update generated notes"
}

initial_minor_case() {
  local dir="$1"
  git -C "$dir" tag -d v1.2.3 >/dev/null
  commit_msg "$dir" "feat: first public release"
}

assert_version "patch" "v1.2.4" "patch" patch_case
assert_version "minor" "v1.3.0" "minor" minor_case
assert_version "major bang" "v2.0.0" "major" major_bang_case
assert_version "major footer" "v2.0.0" "major" major_footer_case
assert_version "none" "" "none" none_case
assert_version "initial minor" "v0.1.0" "minor" initial_minor_case

echo "ok"
