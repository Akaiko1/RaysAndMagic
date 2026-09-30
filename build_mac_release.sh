#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_build_lib.sh
source "${SCRIPT_DIR}/_build_lib.sh"

# Collect with the pinned engine; compile available native shader backends.
go run ./tools/shadergen

APP_NAME="RaysAndMagic"
VIEWER_NAME="RaysAndMagicMapViewer"
OUT_DIR="dist"

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

build_target() {
  local goos="$1"
  local goarch="$2"
  local out_dir="$3"
  local out_name="$4"
  local ldflags="$5"
  local cgo_enabled="$6"
  local package_path="${7:-.}"

  mkdir -p "${out_dir}"
  echo "Building ${package_path} ${goos}/${goarch} -> ${out_dir}/${out_name}"
  CGO_ENABLED="${cgo_enabled}" GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags "${ldflags}" -o "${out_dir}/${out_name}" "${package_path}"
}

# macOS: one universal package (Apple Silicon + Intel). Ebitengine 2.10 uses
# pure Go. Only the game bundle carries the content; the package holds nothing
# else, so the assets ship once.
SLICES_DIR="${OUT_DIR}/.mac_slices"
MAC_DIR="${OUT_DIR}/mac_universal"
for arch in arm64 amd64; do
  build_target darwin "${arch}" "${SLICES_DIR}/${arch}" "${APP_NAME}" "" 0 .
  build_target darwin "${arch}" "${SLICES_DIR}/${arch}" "${VIEWER_NAME}" "" 0 ./assets/map_viewer
done
for name in "${APP_NAME}" "${VIEWER_NAME}"; do
  build_universal_binary "${SLICES_DIR}/${name}" "${SLICES_DIR}/arm64/${name}" "${SLICES_DIR}/amd64/${name}"
done

build_macos_app_bundle "${MAC_DIR}/${APP_NAME}.app"    "${APP_NAME}"    "${SLICES_DIR}/${APP_NAME}"    "com.raysandmagic.game"      "assets/app_icons/rays_and_magic.icns"            content
build_macos_app_bundle "${MAC_DIR}/${VIEWER_NAME}.app" "${VIEWER_NAME}" "${SLICES_DIR}/${VIEWER_NAME}" "com.raysandmagic.mapviewer" "assets/app_icons/rays_and_magic_map_editor.icns" no-content
rm -rf "${SLICES_DIR}"

# Windows (no console window)
build_target windows amd64 "${OUT_DIR}/windows_amd64" "${APP_NAME}.exe" "-H=windowsgui" 0 .
build_target windows amd64 "${OUT_DIR}/windows_amd64" "${VIEWER_NAME}.exe" "-H=windowsgui" 0 ./assets/map_viewer
bundle_runtime_files "${OUT_DIR}/windows_amd64"

echo "Done. Bundled builds in ${OUT_DIR}/"
