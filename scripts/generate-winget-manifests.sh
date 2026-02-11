#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage: $0 --tag <tag> --repo <owner/repo> --checksums <SHA256SUMS.txt> --output-dir <dir>

Example:
  $0 --tag v0.4.0 --repo mingesthq/media-ingest --checksums artifacts/SHA256SUMS.txt --output-dir out/winget
USAGE
}

TAG=""
REPO=""
CHECKSUMS=""
OUT_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      TAG="${2:-}"
      shift 2
      ;;
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    --checksums)
      CHECKSUMS="${2:-}"
      shift 2
      ;;
    --output-dir)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$TAG" || -z "$REPO" || -z "$CHECKSUMS" || -z "$OUT_DIR" ]]; then
  usage
  exit 2
fi

if [[ ! -f "$CHECKSUMS" ]]; then
  echo "checksums file not found: $CHECKSUMS" >&2
  exit 1
fi

VERSION="${TAG#v}"
PKG_ID="Mingest.Mingest"
ASSET_WIN="media-ingest_${TAG}_windows_amd64_bundled.zip"

sha_win="$(awk -v n="$ASSET_WIN" '$2==n {print $1}' "$CHECKSUMS" | head -n 1)"
if [[ -z "$sha_win" ]]; then
  echo "failed to resolve checksum for $ASSET_WIN from $CHECKSUMS" >&2
  exit 1
fi

BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
INSTALLER_URL="${BASE_URL}/${ASSET_WIN}"

DIR="${OUT_DIR}/manifests/m/Mingest/Mingest/${VERSION}"
mkdir -p "$DIR"

cat > "${DIR}/${PKG_ID}.yaml" <<YAML
PackageIdentifier: ${PKG_ID}
PackageVersion: ${VERSION}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
YAML

cat > "${DIR}/${PKG_ID}.installer.yaml" <<YAML
PackageIdentifier: ${PKG_ID}
PackageVersion: ${VERSION}
InstallerType: zip
NestedInstallerType: portable
InstallModes:
- interactive
- silent
- silentWithProgress
Installers:
- Architecture: x64
  InstallerUrl: ${INSTALLER_URL}
  InstallerSha256: ${sha_win}
  NestedInstallerFiles:
  - RelativeFilePath: mingest/mingest.exe
    PortableCommandAlias: mingest
ManifestType: installer
ManifestVersion: 1.6.0
YAML

cat > "${DIR}/${PKG_ID}.locale.en-US.yaml" <<YAML
PackageIdentifier: ${PKG_ID}
PackageVersion: ${VERSION}
PackageLocale: en-US
Publisher: Mingest
PublisherUrl: https://github.com/${REPO}
PublisherSupportUrl: https://github.com/${REPO}/issues
Author: Harrison Wang
PackageName: Mingest
PackageUrl: https://github.com/${REPO}
License: AGPL-3.0-only
LicenseUrl: https://github.com/${REPO}/blob/${TAG}/LICENSE
ShortDescription: Local video archiving CLI powered by yt-dlp and ffmpeg.
Description: Mingest is a local-first video archiving CLI that supports authenticated downloads and mp4-friendly output for personal backups.
Moniker: mingest
Tags:
- video
- downloader
- yt-dlp
- ffmpeg
ManifestType: defaultLocale
ManifestVersion: 1.6.0
YAML

echo "generated winget manifests in: ${DIR}"
