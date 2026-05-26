#!/usr/bin/env bash
#
# Shared helpers sourced by build_bin.sh (local) and build_mac_release.sh
# (distribution). Keeps the .app bundle layout and Info.plist template in
# one place so adding e.g. a CFBundleURLTypes entry only edits one file.
#
# Not executable on its own.

# build_macos_app_bundle assembles a .app directory: copies the binary,
# bundles assets + config.yaml, drops the .icns into Resources, writes a
# minimal Info.plist, and re-signs ad-hoc so Gatekeeper doesn't reject the
# resource seal mismatch (the Go linker pre-signs the bare binary).
#
# Args: app_dir executable_name bin_path bundle_id icon_path
build_macos_app_bundle() {
  local app_dir="$1"
  local executable_name="$2"
  local bin_path="$3"
  local bundle_id="$4"
  local icon_path="$5"

  local contents_dir="${app_dir}/Contents"
  local macos_dir="${contents_dir}/MacOS"
  local resources_dir="${contents_dir}/Resources"
  local icon_name
  icon_name="$(basename "${icon_path}")"

  rm -rf "${app_dir}"
  mkdir -p "${macos_dir}" "${resources_dir}"

  cp "${bin_path}" "${macos_dir}/${executable_name}"
  cp -R assets "${resources_dir}/assets"
  rm -rf "${resources_dir}/assets/map_viewer"
  cp config.yaml "${resources_dir}/config.yaml"
  cp "${icon_path}" "${resources_dir}/${icon_name}"

  cat > "${contents_dir}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>${executable_name}</string>
  <key>CFBundleDisplayName</key>
  <string>${executable_name}</string>
  <key>CFBundleExecutable</key>
  <string>${executable_name}</string>
  <key>CFBundleIdentifier</key>
  <string>${bundle_id}</string>
  <key>CFBundleIconFile</key>
  <string>${icon_name}</string>
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

  # Go's linker auto-signs the bare binary with an ad-hoc signature that
  # claims sealed resources. Once the binary is dropped into the bundle and
  # Resources/ is populated, that signature no longer matches and Gatekeeper
  # silently refuses to launch the .app. Strip and re-sign over the full
  # bundle so the resource seal is correct.
  if command -v codesign >/dev/null 2>&1; then
    codesign --remove-signature "${macos_dir}/${executable_name}" 2>/dev/null || true
    codesign --force --deep --sign - "${app_dir}"
  fi
}
