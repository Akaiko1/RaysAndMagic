#!/usr/bin/env bash
set -euo pipefail

APP_NAME="RaysAndMagic"
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

  mkdir -p "${out_dir}"
  echo "Building ${goos}/${goarch} -> ${out_dir}/${out_name}"
  CGO_ENABLED="${cgo_enabled}" GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags "${ldflags}" -o "${out_dir}/${out_name}" .

  # Bundle runtime assets/config next to the binary
  cp -R assets "${out_dir}/assets"
  cp config.yaml "${out_dir}/config.yaml"
}

# macOS (Intel + Apple Silicon) - Ebiten needs cgo on macOS
build_target darwin amd64 "${OUT_DIR}/mac_amd64" "${APP_NAME}" "" 1
build_target darwin arm64 "${OUT_DIR}/mac_arm64" "${APP_NAME}" "" 1

# Windows (no console window)
build_target windows amd64 "${OUT_DIR}/windows_amd64" "${APP_NAME}.exe" "-H=windowsgui" 0

echo "Done. Bundled builds in ${OUT_DIR}/"
