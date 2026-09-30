#!/usr/bin/env bash
#
# Shared helpers sourced by build_bin.sh (local) and build_mac_release.sh
# (distribution). Keeps the .app bundle layout and Info.plist template in
# one place so adding e.g. a CFBundleURLTypes entry only edits one file.
#
# Not executable on its own.

# Oldest macOS the bundles support: the Go toolchain's darwin floor (Go 1.25:
# macOS 12). Raise it together with the toolchain.
MACOS_MIN_VERSION="12.0"

# assert_macos_min_version fails the build when a binary (every slice of a
# universal one) needs a newer macOS than MACOS_MIN_VERSION. A cgo build links
# against the host SDK and inherits the build machine's version, which
# silently limits who can run it.
assert_macos_min_version() {
  local bin_path="$1"
  if ! command -v otool >/dev/null 2>&1; then
    echo "warning: otool missing, cannot check the macOS floor of ${bin_path}" >&2
    return 0
  fi
  local versions minos
  versions="$(otool -l -arch all "${bin_path}" | awk '$1=="minos"{print $2}')"
  if [[ -z "${versions}" ]]; then
    echo "error: ${bin_path} carries no LC_BUILD_VERSION minimum macOS" >&2
    return 1
  fi
  while read -r minos; do
    if awk -v got="${minos}" -v want="${MACOS_MIN_VERSION}" 'BEGIN {
      split(got, g, "."); split(want, w, ".")
      exit !((g[1] + 0 > w[1] + 0) || (g[1] + 0 == w[1] + 0 && g[2] + 0 > w[2] + 0))
    }'; then
      echo "error: ${bin_path} requires macOS ${minos}, bundles promise ${MACOS_MIN_VERSION} (build with CGO_ENABLED=0)" >&2
      return 1
    fi
  done <<< "${versions}"
}

# build_universal_binary joins an arm64 and an amd64 build into one binary that
# runs natively on Apple Silicon and Intel Macs.
build_universal_binary() {
  local out_path="$1"
  local arm64_path="$2"
  local amd64_path="$3"
  mkdir -p "$(dirname "${out_path}")"
  lipo -create -output "${out_path}" "${arm64_path}" "${amd64_path}"
}

# Runtime packages contain authored assets, never Go packages or editor sources.
# Refresh the owned asset directory so repeated builds cannot retain old files.
bundle_runtime_files() {
  local out_dir="$1"
  mkdir -p "${out_dir}"
  rm -rf "${out_dir}/assets"
  cp -R assets "${out_dir}/assets"
  rm -rf "${out_dir}/assets/map_viewer"
  find "${out_dir}/assets" -type f -name '*.go' -delete
  cp config.yaml "${out_dir}/config.yaml"
}

# build_macos_app_bundle assembles a .app directory: copies the binary,
# bundles assets + config.yaml (content="content"), drops the .icns into
# Resources, writes a minimal Info.plist, and re-signs ad-hoc so Gatekeeper
# doesn't reject the resource seal mismatch (the Go linker pre-signs the bare
# binary). Only the game bundle carries content; the editor bundle
# (content="no-content") reads the game's at runtime (internal/storage).
#
# Args: app_dir executable_name bin_path bundle_id icon_path content
build_macos_app_bundle() {
  local app_dir="$1"
  local executable_name="$2"
  local bin_path="$3"
  local bundle_id="$4"
  local icon_path="$5"
  local content="$6"

  local contents_dir="${app_dir}/Contents"
  local macos_dir="${contents_dir}/MacOS"
  local resources_dir="${contents_dir}/Resources"
  local icon_name
  icon_name="$(basename "${icon_path}")"

  assert_macos_min_version "${bin_path}"
  rm -rf "${app_dir}"
  mkdir -p "${macos_dir}" "${resources_dir}"

  cp "${bin_path}" "${macos_dir}/${executable_name}"
  case "${content}" in
    content) bundle_runtime_files "${resources_dir}" ;;
    no-content) ;;
    *)
      echo "error: build_macos_app_bundle content must be content or no-content, got '${content}'" >&2
      return 1
      ;;
  esac
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
  <string>${MACOS_MIN_VERSION}</string>
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
