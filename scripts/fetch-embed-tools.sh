#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
fetch-embed-tools.sh

Downloads tool binaries into embed/<goos>/ for optional embedding (build tag: embedtools).

Examples:
  scripts/fetch-embed-tools.sh --os windows --arch amd64
  scripts/fetch-embed-tools.sh --os darwin --arch arm64 --tools yt-dlp,ffmpeg,deno

Flags:
  --os <goos>        windows|linux|darwin (default: GOOS env or `go env GOOS`)
  --arch <goarch>    amd64|arm64         (default: GOARCH env or `go env GOARCH`)
  --tools <csv>      yt-dlp,ffmpeg,deno  (default: yt-dlp,ffmpeg,deno)
EOF
}

die() { echo "error: $*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

csv_has() {
  local csv="$1"
  local want="$2"
  IFS=',' read -r -a parts <<<"$csv"
  for p in "${parts[@]}"; do
    if [[ "${p}" == "${want}" ]]; then
      return 0
    fi
  done
  return 1
}

tmpdir=""
cleanup() {
  if [[ -n "${tmpdir}" && -d "${tmpdir}" ]]; then
    rm -rf "${tmpdir}"
  fi
}
trap cleanup EXIT

GOOS="${GOOS:-}"
GOARCH="${GOARCH:-}"
TOOLS="yt-dlp,ffmpeg,deno"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --os) GOOS="${2:-}"; shift 2 ;;
    --arch) GOARCH="${2:-}"; shift 2 ;;
    --tools) TOOLS="${2:-}"; shift 2 ;;
    *) die "unknown arg: $1 (use --help)" ;;
  esac
done

need curl

PYTHON="${PYTHON:-}"
if [[ -z "${PYTHON}" ]]; then
  if command -v python3 >/dev/null 2>&1; then
    PYTHON="python3"
  elif command -v python >/dev/null 2>&1; then
    PYTHON="python"
  else
    die "missing required command: python3 (or python)"
  fi
fi

if [[ -z "${GOOS}" ]]; then GOOS="$(go env GOOS)"; fi
if [[ -z "${GOARCH}" ]]; then GOARCH="$(go env GOARCH)"; fi

case "${GOOS}" in
  windows|linux|darwin) ;;
  *) die "unsupported --os: ${GOOS}" ;;
esac

case "${GOARCH}" in
  amd64|arm64) ;;
  *) die "unsupported --arch: ${GOARCH}" ;;
esac

# Only required for extracting linux ffmpeg tarballs.
if csv_has "${TOOLS}" "ffmpeg" && [[ "${GOOS}" == "linux" ]]; then
  need tar
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Embedding assets live next to the embedtools package so go:embed patterns are simple.
out_dir="${root}/ingest/embedtools/assets/${GOOS}"
mkdir -p "${out_dir}"

tmpdir="$(mktemp -d)"

download() {
  local url="$1"
  local out="$2"
  echo "download: ${url}"
  # `--retry-all-errors` helps with transient DNS and GitHub edge hiccups.
  curl -fsSL --retry 6 --retry-all-errors --retry-delay 2 --connect-timeout 20 -o "${out}" "${url}"
}

extract_zip_member() {
  local zip="$1"
  local member_suffix="$2"
  local out="$3"
  "${PYTHON}" - "${zip}" "${member_suffix}" "${out}" <<'PY'
import os, sys, zipfile
zip_path, suffix, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
with zipfile.ZipFile(zip_path) as z:
  names = z.namelist()
  cands = [n for n in names if n.endswith(suffix) and not n.endswith("/")]
  if not cands:
    raise SystemExit(f"zip member not found (suffix={suffix}): {zip_path}")
  # Prefer shortest path (usually the canonical bin/<file>).
  cands.sort(key=len)
  name = cands[0]
  os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
  with z.open(name) as src, open(out_path, "wb") as dst:
    dst.write(src.read())
print(f"extract: {name} -> {out_path}")
PY
}

chmod_x() {
  local path="$1"
  if [[ "${GOOS}" != "windows" ]]; then
    chmod 0755 "${path}"
  fi
}

fetch_ytdlp() {
  local target="${out_dir}/yt-dlp"
  if [[ "${GOOS}" == "windows" ]]; then
    target="${out_dir}/yt-dlp.exe"
    download "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe" "${target}"
  else
    # Unix executable (python zipapp / script); works on linux+darwin.
    download "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp" "${target}"
  fi
  chmod_x "${target}"
}

fetch_deno() {
  local zip="${tmpdir}/deno.zip"
  local target="${out_dir}/deno"

  local asset=""
  case "${GOOS}/${GOARCH}" in
    windows/amd64) asset="deno-x86_64-pc-windows-msvc.zip"; target="${out_dir}/deno.exe" ;;
    linux/amd64) asset="deno-x86_64-unknown-linux-gnu.zip" ;;
    linux/arm64) asset="deno-aarch64-unknown-linux-gnu.zip" ;;
    darwin/amd64) asset="deno-x86_64-apple-darwin.zip" ;;
    darwin/arm64) asset="deno-aarch64-apple-darwin.zip" ;;
    *) die "no deno asset mapping for ${GOOS}/${GOARCH}" ;;
  esac

  download "https://github.com/denoland/deno/releases/latest/download/${asset}" "${zip}"
  extract_zip_member "${zip}" "$(basename "${target}")" "${target}"
  chmod_x "${target}"
}

fetch_ffmpeg() {
  local archive="${tmpdir}/ffmpeg"
  local target="${out_dir}/ffmpeg"

  if [[ "${GOOS}" == "windows" ]]; then
    target="${out_dir}/ffmpeg.exe"
    archive="${archive}.zip"
    # Static (non-shared) Windows build.
    download "https://github.com/BtbN/FFmpeg-Builds/releases/latest/download/ffmpeg-master-latest-win64-gpl.zip" "${archive}"
    extract_zip_member "${archive}" "bin/ffmpeg.exe" "${target}"
  elif [[ "${GOOS}" == "darwin" ]]; then
    # BtbN/FFmpeg-Builds does not ship macOS artifacts. Use Martin Riedl's signed builds.
    # Scripting URLs documented on https://ffmpeg.martin-riedl.de/.
    archive="${archive}.zip"
    local arch="${GOARCH}"
    download "https://ffmpeg.martin-riedl.de/redirect/latest/macos/${arch}/release/ffmpeg.zip" "${archive}"
    extract_zip_member "${archive}" "ffmpeg" "${target}"
    chmod_x "${target}"
  else
    local platform=""
    case "${GOOS}/${GOARCH}" in
      linux/amd64) platform="linux64" ;;
      linux/arm64) platform="linuxarm64" ;;
      *) die "no ffmpeg asset mapping for ${GOOS}/${GOARCH}" ;;
    esac

    archive="${archive}.tar.xz"
    download "https://github.com/BtbN/FFmpeg-Builds/releases/latest/download/ffmpeg-master-latest-${platform}-gpl.tar.xz" "${archive}"

    local xdir="${tmpdir}/ffmpeg-extract"
    mkdir -p "${xdir}"
    tar -C "${xdir}" -xf "${archive}"

    local found=""
    found="$(find "${xdir}" -type f -name ffmpeg -perm -u+x 2>/dev/null | head -n 1 || true)"
    if [[ -z "${found}" ]]; then
      found="$(find "${xdir}" -type f -name ffmpeg 2>/dev/null | head -n 1 || true)"
    fi
    [[ -n "${found}" ]] || die "ffmpeg binary not found inside: ${archive}"
    cp -f "${found}" "${target}"
    chmod_x "${target}"
  fi
}

echo "target: ${out_dir}"
echo "tools:  ${TOOLS}"

if csv_has "${TOOLS}" "yt-dlp"; then fetch_ytdlp; fi
if csv_has "${TOOLS}" "deno"; then fetch_deno; fi
if csv_has "${TOOLS}" "ffmpeg"; then fetch_ffmpeg; fi

echo "ok:"
ls -la "${out_dir}"
