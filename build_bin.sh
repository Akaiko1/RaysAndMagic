#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_build_lib.sh
source "${SCRIPT_DIR}/_build_lib.sh"

APP_NAME="RaysAndMagic"
VIEWER_NAME="RaysAndMagicMapViewer"
BIN_DIR="bin"

mkdir -p "${BIN_DIR}"

go build -o "${BIN_DIR}/raysandmagic" .
go build -o "${BIN_DIR}/map_viewer" ./assets/map_viewer

build_macos_app_bundle "${BIN_DIR}/${APP_NAME}.app"        "${APP_NAME}"    "${BIN_DIR}/raysandmagic" "com.raysandmagic.game"      "assets/app_icons/rays_and_magic.icns"
build_macos_app_bundle "${BIN_DIR}/${VIEWER_NAME}.app"     "${VIEWER_NAME}" "${BIN_DIR}/map_viewer"   "com.raysandmagic.mapviewer" "assets/app_icons/rays_and_magic_map_editor.icns"

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-H=windowsgui" -o "${BIN_DIR}/${APP_NAME}.exe" .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-H=windowsgui" -o "${BIN_DIR}/${VIEWER_NAME}.exe" ./assets/map_viewer

echo "Built local binaries and icon-bearing app artifacts in ${BIN_DIR}/"
