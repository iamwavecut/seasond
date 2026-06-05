#!/usr/bin/env bash
set -euo pipefail

latest_tag="$(
  git tag --merged HEAD --sort=-v:refname |
    grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' |
    head -n 1 || true
)"

range=HEAD
base_version="0.0.0"
if [[ -n "$latest_tag" ]]; then
  range="${latest_tag}..HEAD"
  base_version="${latest_tag#v}"
fi

log_file="$(mktemp)"
trap 'rm -f "$log_file"' EXIT
git log --format='%s%n%b%n---END-COMMIT---' "$range" >"$log_file"

release_type="none"
if grep -Eq '^[[:alnum:]_-]+(\([^)]+\))?!:' "$log_file" ||
  grep -Eq '^BREAKING[ -]CHANGE:' "$log_file"; then
  release_type="major"
elif grep -Eq '^feat(\([^)]+\))?:' "$log_file"; then
  release_type="minor"
elif grep -Eq '^(fix|perf|refactor|build|ci|chore|docs|style|test|revert)(\([^)]+\))?:' "$log_file"; then
  release_type="patch"
fi

version=""
if [[ "$release_type" != "none" ]]; then
  IFS=. read -r major minor patch <<<"$base_version"
  case "$release_type" in
    major)
      major=$((major + 1))
      minor=0
      patch=0
      ;;
    minor)
      minor=$((minor + 1))
      patch=0
      ;;
    patch)
      patch=$((patch + 1))
      ;;
  esac
  version="v${major}.${minor}.${patch}"
fi

version_no_v="${version#v}"
if [[ -z "$version" ]]; then
  version_no_v=""
fi

echo "version=$version"
echo "version_no_v=$version_no_v"
echo "release_type=$release_type"
echo "previous_tag=$latest_tag"
