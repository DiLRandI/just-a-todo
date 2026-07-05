#!/usr/bin/env sh

set -euo pipefail

OWNER="DiLRandI"
REPO="just-a-todo"
BINARY="todo"
API_URL="https://api.github.com/repos/${OWNER}/${REPO}"

VERSION="${VERSION:-latest}"
INSTALL_DIR="${PREFIX:-${TODO_INSTALL_DIR:-}}"

usage() {
  cat <<'EOF_USAGE'
Usage: ./install.sh [VERSION] [options]

Install latest release:
  ./install.sh

Install a specific version:
  ./install.sh v0.3.0
  ./install.sh 0.3.0
  VERSION=0.3.0 ./install.sh

Options:
  --prefix PATH       Install directory (overrides PREFIX/TODO_INSTALL_DIR)
  --install-dir PATH  Same as --prefix
  --help              Show this help

Environment:
  PREFIX              Install directory (optional)
  TODO_INSTALL_DIR    Install directory (optional)
  VERSION             Release tag to install (default: latest)
EOF_USAGE
}

error() {
  echo "error: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "$1 is required"
}

normalize_lower() {
  tr '[:upper:]' '[:lower:]'
}

require_cmd curl

while [ "$#" -gt 0 ]; do
  case "$1" in
    --prefix|--install-dir)
      if [ "$#" -lt 2 ]; then
        error "$1 requires a directory value"
      fi
      INSTALL_DIR="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --*)
      error "unknown option: $1"
      ;;
    *)
      if [ "$VERSION" = "latest" ]; then
        VERSION="$1"
      else
        error "unexpected argument: $1"
      fi
      shift
      ;;
  esac
done

if [ "$VERSION" = "latest" ]; then
  release_url="${API_URL}/releases/latest"
else
  # GitHub tags are prefixed with `v` in this repo.
  tag="${VERSION#v}"
  release_url="${API_URL}/releases/tags/v${tag}"
fi

release_json="$(curl -fsSL "$release_url")"
tag_name="$(printf '%s' "$release_json" | tr '\n' ' ' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
[ -n "$tag_name" ] || error "could not determine release tag from GitHub API"
tag_without_v="${tag_name#v}"

asset_urls="$(printf '%s' "$release_json" | tr '\n' ' ' | grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*"' | sed -E 's/"browser_download_url"[[:space:]]*:[[:space:]]*"([^"]*)"/\\1/')"
[ -n "$asset_urls" ] || error "no assets found for ${tag_name}"

checksum_url="$(printf '%s\n' "$asset_urls" | grep -i 'checksums.txt$' | head -n1 || true)"

case "$(uname -s)" in
  Linux*) os="linux" ;;
  Darwin*) os="darwin" ;;
  *MSYS*|*MINGW*|*CYGWIN*|Windows_NT*) os="windows" ;;
  *) error "unsupported OS: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) error "unsupported architecture: $(uname -m)" ;;
esac

if [ "$os" = "windows" ]; then
  ext="zip"
  bin_ext=".exe"
else
  ext="tar.gz"
  bin_ext=""
fi

if [ "$arch" = "amd64" ]; then
  arch_candidates="amd64 x86_64"
else
  arch_candidates="$arch"
fi

archive_url=""
archive_file_name=""

while IFS= read -r url; do
  for tag in "$tag_name" "$tag_without_v"; do
    for arch_candidate in $arch_candidates; do
      candidate_pattern="${BINARY}_${tag}_${os}_${arch_candidate}.${ext}"
      normalized="$(printf '%s' "$url" | normalize_lower)"
      if printf '%s\n' "$normalized" | grep -q "$candidate_pattern"; then
        archive_url="$url"
        archive_file_name="$(printf '%s' "$url" | awk -F/ '{print $NF}')"
        break 3
      fi
    done
  done
done <<EOF_ASSETS
$(printf '%s\n' "$asset_urls")
EOF_ASSETS

if [ -z "$archive_url" ]; then
  while IFS= read -r url; do
    for tag in "$tag_name" "$tag_without_v"; do
      for arch_candidate in $arch_candidates; do
        normalized="$(printf '%s' "$url" | normalize_lower)"
        if printf '%s\n' "$normalized" | grep -iq "${BINARY}.*${tag}.*${os}.*${arch_candidate}.*${ext}"; then
          archive_url="$url"
          archive_file_name="$(printf '%s' "$url" | awk -F/ '{print $NF}')"
          break 3
        fi
      done
    done
  done <<EOF_ASSETS
$(printf '%s\n' "$asset_urls")
EOF_ASSETS
fi

if [ -z "$archive_url" ]; then
  error "could not find matching asset for ${os}/${arch} in release ${tag_name}"
fi

if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

mkdir -p "$INSTALL_DIR"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
archive_path="${tmp_dir}/${archive_file_name}"
checksum_path="${tmp_dir}/checksums.txt"
extract_dir="${tmp_dir}/extract"
mkdir -p "$extract_dir"

curl -fLo "$archive_path" "$archive_url"

if [ "$os" = "windows" ]; then
  require_cmd unzip
  unzip -q "$archive_path" -d "$extract_dir"
else
  require_cmd tar
  tar -xzf "$archive_path" -C "$extract_dir"
fi

installed_binary="$(find "$extract_dir" -type f -name "${BINARY}${bin_ext}" | head -n 1)"
[ -n "$installed_binary" ] || error "could not locate ${BINARY}${bin_ext} in archive"

if [ -n "$checksum_url" ]; then
  curl -fLo "$checksum_path" "$checksum_url"

  expected_sum="$(awk -v file="$archive_file_name" '
    {
      file_name=$NF
      if (file_name == file || file_name == "./" file || file_name == "*" file || file_name == "*./" file) {
        print $1
        exit
      }
    }
  ' "$checksum_path" | head -n1 || true)"

  computed_sum=""
  if command -v sha256sum >/dev/null 2>&1; then
    computed_sum="$(sha256sum "$archive_path" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    computed_sum="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
  fi

  if [ -n "$expected_sum" ] && [ -n "$computed_sum" ] && [ "$computed_sum" != "$expected_sum" ]; then
    error "checksum mismatch for $archive_file_name"
  elif [ -n "$checksum_url" ] && [ -z "$expected_sum" ]; then
    echo "warning: no checksum entry found in checksums.txt for $archive_file_name"
  elif [ -z "$computed_sum" ]; then
    echo "warning: no sha256 utility found; skipping checksum verification"
  fi
fi

install_path="${INSTALL_DIR}/${BINARY}${bin_ext}"
cp "$installed_binary" "$install_path"

if [ "$os" != "windows" ]; then
  chmod +x "$install_path"
fi

echo "installed ${BINARY}${bin_ext} to ${install_path}"
echo "version: ${tag_name}"
echo
echo "to ensure PATH, run:"
echo "  export PATH=\"${INSTALL_DIR}:\\$PATH\""
