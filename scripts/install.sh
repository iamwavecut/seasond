#!/usr/bin/env bash
set -euo pipefail

repo="iamwavecut/seasond"
binary="seasond"
default_install_dir="${HOME}/.local/bin"
install_dir="${INSTALL_DIR:-}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: $1 is required" >&2
    exit 1
  fi
}

detect_platform() {
  local os arch
  case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="macos" ;;
    *) echo "error: unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac

  case "$(uname -m)" in
    x86_64 | amd64) arch="x64" ;;
    i386 | i686) arch="x86" ;;
    arm64 | aarch64)
      if [[ "$os" == "macos" ]]; then
        arch="arm64"
      else
        echo "error: linux arm64 archive is not published yet" >&2
        exit 1
      fi
      ;;
    *) echo "error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac

  echo "${os}_${arch}"
}

latest_version() {
  curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" |
    sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' |
    head -n 1
}

choose_install_dir() {
  if [[ -n "$install_dir" ]]; then
    echo "$install_dir"
    return
  fi
  if [[ -r /dev/tty ]]; then
    local answer
    read -r -p "Install directory [${default_install_dir}]: " answer </dev/tty
    echo "${answer:-$default_install_dir}"
    return
  fi
  echo "$default_install_dir"
}

install_binary() {
  local src="$1"
  local dst_dir="$2"
  local dst="${dst_dir}/${binary}"

  if [[ -w "$dst_dir" ]]; then
    install -m 0755 "$src" "$dst"
    return
  fi

  if command -v sudo >/dev/null 2>&1; then
    sudo install -m 0755 "$src" "$dst"
    return
  fi

  echo "error: ${dst_dir} is not writable and sudo is unavailable" >&2
  exit 1
}

main() {
  need curl
  need tar

  local platform version archive url tmp install_to
  platform="$(detect_platform)"
  version="${VERSION:-$(latest_version)}"
  if [[ -z "$version" ]]; then
    echo "error: could not resolve latest release version" >&2
    exit 1
  fi

  archive="seasond_${version}_${platform}.tar.gz"
  url="https://github.com/${repo}/releases/download/${version}/${archive}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  curl -fL "$url" -o "$tmp/$archive"
  tar -xzf "$tmp/$archive" -C "$tmp"

  install_to="$(choose_install_dir)"
  mkdir -p "$install_to"
  install_binary "$tmp/$binary" "$install_to"

  echo "Installed ${binary} ${version} to ${install_to}/${binary}"
  case ":$PATH:" in
    *":$install_to:"*) ;;
    *) echo "Note: ${install_to} is not in PATH." ;;
  esac
}

main "$@"
