#!/usr/bin/env bash
set -euo pipefail

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

build_macos_app() {
  local arch_label="$1"
  local bin_path="$2"
  local app_dir="${OUT_DIR}/mac_${arch_label}/${APP_NAME}.app"
  local contents_dir="${app_dir}/Contents"
  local macos_dir="${contents_dir}/MacOS"
  local resources_dir="${contents_dir}/Resources"

  rm -rf "${app_dir}"
  mkdir -p "${macos_dir}" "${resources_dir}"

  cp "${bin_path}" "${macos_dir}/${APP_NAME}"
  cp -R assets "${resources_dir}/assets"
  rm -rf "${resources_dir}/assets/map_viewer"
  cp config.yaml "${resources_dir}/config.yaml"

  cat > "${contents_dir}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>${APP_NAME}</string>
  <key>CFBundleDisplayName</key>
  <string>${APP_NAME}</string>
  <key>CFBundleExecutable</key>
  <string>${APP_NAME}</string>
  <key>CFBundleIdentifier</key>
  <string>com.raysandmagic.game</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleVersion</key>
  <string>1.0</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.13</string>
</dict>
</plist>
EOF
}

# macOS (Intel + Apple Silicon) - Ebiten needs cgo on macOS
build_target darwin amd64 "${OUT_DIR}/mac_amd64" "${APP_NAME}" "" 1 .
build_target darwin amd64 "${OUT_DIR}/mac_amd64" "${VIEWER_NAME}" "" 1 ./assets/map_viewer
bundle_runtime_files "${OUT_DIR}/mac_amd64"

build_target darwin arm64 "${OUT_DIR}/mac_arm64" "${APP_NAME}" "" 1 .
build_target darwin arm64 "${OUT_DIR}/mac_arm64" "${VIEWER_NAME}" "" 1 ./assets/map_viewer
bundle_runtime_files "${OUT_DIR}/mac_arm64"

build_macos_app "amd64" "${OUT_DIR}/mac_amd64/${APP_NAME}"
build_macos_app "arm64" "${OUT_DIR}/mac_arm64/${APP_NAME}"

# Windows (no console window)
build_target windows amd64 "${OUT_DIR}/windows_amd64" "${APP_NAME}.exe" "-H=windowsgui" 0 .
build_target windows amd64 "${OUT_DIR}/windows_amd64" "${VIEWER_NAME}.exe" "-H=windowsgui" 0 ./assets/map_viewer
bundle_runtime_files "${OUT_DIR}/windows_amd64"

echo "Done. Bundled builds in ${OUT_DIR}/"
