#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_build_lib.sh
source "${SCRIPT_DIR}/_build_lib.sh"

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

bundle_runtime_files() {
  local out_dir="$1"
  cp -R assets "${out_dir}/assets"
  rm -rf "${out_dir}/assets/map_viewer"
  cp config.yaml "${out_dir}/config.yaml"
}

# macOS (Intel + Apple Silicon) - Ebiten needs cgo on macOS
build_target darwin amd64 "${OUT_DIR}/mac_amd64" "${APP_NAME}" "" 1 .
build_target darwin amd64 "${OUT_DIR}/mac_amd64" "${VIEWER_NAME}" "" 1 ./assets/map_viewer
bundle_runtime_files "${OUT_DIR}/mac_amd64"

build_target darwin arm64 "${OUT_DIR}/mac_arm64" "${APP_NAME}" "" 1 .
build_target darwin arm64 "${OUT_DIR}/mac_arm64" "${VIEWER_NAME}" "" 1 ./assets/map_viewer
bundle_runtime_files "${OUT_DIR}/mac_arm64"

build_macos_app_bundle "${OUT_DIR}/mac_amd64/${APP_NAME}.app"    "${APP_NAME}"    "${OUT_DIR}/mac_amd64/${APP_NAME}"    "com.raysandmagic.game"      "assets/app_icons/rays_and_magic.icns"
build_macos_app_bundle "${OUT_DIR}/mac_amd64/${VIEWER_NAME}.app" "${VIEWER_NAME}" "${OUT_DIR}/mac_amd64/${VIEWER_NAME}" "com.raysandmagic.mapviewer" "assets/app_icons/rays_and_magic_map_editor.icns"
build_macos_app_bundle "${OUT_DIR}/mac_arm64/${APP_NAME}.app"    "${APP_NAME}"    "${OUT_DIR}/mac_arm64/${APP_NAME}"    "com.raysandmagic.game"      "assets/app_icons/rays_and_magic.icns"
build_macos_app_bundle "${OUT_DIR}/mac_arm64/${VIEWER_NAME}.app" "${VIEWER_NAME}" "${OUT_DIR}/mac_arm64/${VIEWER_NAME}" "com.raysandmagic.mapviewer" "assets/app_icons/rays_and_magic_map_editor.icns"

# Windows (no console window)
build_target windows amd64 "${OUT_DIR}/windows_amd64" "${APP_NAME}.exe" "-H=windowsgui" 0 .
build_target windows amd64 "${OUT_DIR}/windows_amd64" "${VIEWER_NAME}.exe" "-H=windowsgui" 0 ./assets/map_viewer
bundle_runtime_files "${OUT_DIR}/windows_amd64"

echo "Done. Bundled builds in ${OUT_DIR}/"
