#!/bin/sh
# fleet installer: resolves the latest GitHub release and installs the matching
# prebuilt binary. Usage:
#   curl -fsSL https://raw.githubusercontent.com/zairedegrees/fleet/main/scripts/install.sh | sh
set -eu

REPO="zairedegrees/fleet"

err() { echo "fleet-install: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "required tool not found: $1"; }

main() {
  need curl
  need tar

  os=$(uname -s)
  arch=$(uname -m)
  case "$os" in
    Darwin) goos="darwin" ;;
    Linux)  goos="linux" ;;
    *) err "unsupported OS: $os (fleet ships macOS and Linux binaries)" ;;
  esac
  case "$arch" in
    x86_64|amd64) goarch="amd64" ;;
    arm64|aarch64) goarch="arm64" ;;
    *) err "unsupported architecture: $arch" ;;
  esac

  tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name":' | head -1 | sed -E 's/.*"tag_name" *: *"([^"]+)".*/\1/')
  [ -n "$tag" ] || err "could not resolve the latest release tag"

  asset="fleet_${goos}_${goarch}.tar.gz"
  base="https://github.com/${REPO}/releases/download/${tag}"

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  echo "fleet-install: downloading ${asset} (${tag})"
  curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}" || err "download failed: ${asset}"
  curl -fsSL "${base}/checksums.txt" -o "${tmp}/checksums.txt" || err "download failed: checksums.txt"

  echo "fleet-install: verifying checksum"
  (
    cd "$tmp"
    line=$(grep " ${asset}\$" checksums.txt) || exit 1
    if command -v sha256sum >/dev/null 2>&1; then
      echo "$line" | sha256sum -c -
    else
      echo "$line" | shasum -a 256 -c -
    fi
  ) >/dev/null 2>&1 || err "checksum verification failed for ${asset}"

  tar -xzf "${tmp}/${asset}" -C "$tmp"
  [ -f "${tmp}/fleet" ] || err "archive did not contain a fleet binary"

  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dest="/usr/local/bin"
  else
    dest="${HOME}/.local/bin"
    mkdir -p "$dest"
  fi
  install -m 0755 "${tmp}/fleet" "${dest}/fleet"

  if [ "$goos" = "darwin" ]; then
    xattr -d com.apple.quarantine "${dest}/fleet" 2>/dev/null || true
  fi

  echo "fleet-install: installed ${tag} to ${dest}/fleet"
  case ":${PATH}:" in
    *":${dest}:"*) ;;
    *) echo "fleet-install: add ${dest} to your PATH:  export PATH=\"${dest}:\$PATH\"" ;;
  esac
  "${dest}/fleet" --version || true
}

main "$@"
