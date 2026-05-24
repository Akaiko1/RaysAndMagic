#!/usr/bin/env bash
#
# Regenerates platform icon bundles from the master PNGs in assets/app_icons/.
#
# Tools required:
#   - iconutil, sips                (macOS built-ins)
#   - github.com/akavel/rsrc        (go install github.com/akavel/rsrc@latest)
#
# Inputs (must exist before running):
#   assets/app_icons/rays_and_magic_icon.png             (master, ≥ 1024×1024)
#   assets/app_icons/rays_and_magic_map_editor_icon.png  (master, ≥ 1024×1024)
#   assets/app_icons/rays_and_magic.ico                  (Windows icon)
#   assets/app_icons/rays_and_magic_map_editor.ico       (Windows icon)
#
# Outputs (overwritten on each run):
#   assets/app_icons/<name>.iconset/   (intermediate; .gitignored)
#   assets/app_icons/<name>.icns       (macOS app bundle icon)
#   rsrc_windows_amd64.syso            (game; auto-linked by `go build`)
#   assets/map_viewer/rsrc_windows_amd64.syso (viewer; same)
#
# Note on .ico: macOS has no first-party PNG→ICO converter. The .ico files
# are produced externally (ImageMagick: `magick input.png -define
# icon:auto-resize=16,32,48,64,128,256 out.ico`, or any online converter)
# and committed to the repo alongside the master PNG. Update them manually
# if the master art changes; this script only rebuilds the .syso wrapper.

set -euo pipefail

ICONS_DIR="assets/app_icons"
VIEWER_PKG_DIR="assets/map_viewer"

# Sizes that iconutil expects in an .iconset directory.
ICONSET_SIZES=(16 32 64 128 256 512 1024)

require_tool() {
  local name="$1"
  local install_hint="$2"
  if ! command -v "${name}" >/dev/null 2>&1; then
    echo "error: required tool '${name}' not found. ${install_hint}" >&2
    exit 1
  fi
}

require_input() {
  local path="$1"
  if [[ ! -f "${path}" ]]; then
    echo "error: missing input '${path}'" >&2
    exit 1
  fi
}

build_iconset_and_icns() {
  local master_png="$1"
  local icon_basename="$2"
  local iconset_dir="${ICONS_DIR}/${icon_basename}.iconset"
  local icns_path="${ICONS_DIR}/${icon_basename}.icns"

  echo "==> building ${icns_path}"
  rm -rf "${iconset_dir}"
  mkdir -p "${iconset_dir}"

  # iconutil requires this exact filename pattern. @2x sizes use the next
  # larger sips render so retina displays get crisp art.
  sips -z 16   16   "${master_png}" --out "${iconset_dir}/icon_16x16.png"      >/dev/null
  sips -z 32   32   "${master_png}" --out "${iconset_dir}/icon_16x16@2x.png"   >/dev/null
  sips -z 32   32   "${master_png}" --out "${iconset_dir}/icon_32x32.png"      >/dev/null
  sips -z 64   64   "${master_png}" --out "${iconset_dir}/icon_32x32@2x.png"   >/dev/null
  sips -z 128  128  "${master_png}" --out "${iconset_dir}/icon_128x128.png"    >/dev/null
  sips -z 256  256  "${master_png}" --out "${iconset_dir}/icon_128x128@2x.png" >/dev/null
  sips -z 256  256  "${master_png}" --out "${iconset_dir}/icon_256x256.png"    >/dev/null
  sips -z 512  512  "${master_png}" --out "${iconset_dir}/icon_256x256@2x.png" >/dev/null
  sips -z 512  512  "${master_png}" --out "${iconset_dir}/icon_512x512.png"    >/dev/null
  sips -z 1024 1024 "${master_png}" --out "${iconset_dir}/icon_512x512@2x.png" >/dev/null

  iconutil --convert icns "${iconset_dir}" --output "${icns_path}"
}

build_windows_syso() {
  local ico_path="$1"
  local out_path="$2"
  echo "==> building ${out_path}"
  rsrc -ico "${ico_path}" -arch amd64 -o "${out_path}"
}

# Pick up tools installed via `go install` even if GOPATH/bin isn't on PATH.
if ! command -v rsrc >/dev/null 2>&1 && [[ -x "${HOME}/go/bin/rsrc" ]]; then
  PATH="${HOME}/go/bin:${PATH}"
fi

require_tool iconutil "ships with macOS"
require_tool sips     "ships with macOS"
require_tool rsrc     "install with: go install github.com/akavel/rsrc@latest"

require_input "${ICONS_DIR}/rays_and_magic_icon.png"
require_input "${ICONS_DIR}/rays_and_magic_map_editor_icon.png"
require_input "${ICONS_DIR}/rays_and_magic.ico"
require_input "${ICONS_DIR}/rays_and_magic_map_editor.ico"

build_iconset_and_icns "${ICONS_DIR}/rays_and_magic_icon.png"            "rays_and_magic"
build_iconset_and_icns "${ICONS_DIR}/rays_and_magic_map_editor_icon.png" "rays_and_magic_map_editor"

build_windows_syso "${ICONS_DIR}/rays_and_magic.ico"            "rsrc_windows_amd64.syso"
build_windows_syso "${ICONS_DIR}/rays_and_magic_map_editor.ico" "${VIEWER_PKG_DIR}/rsrc_windows_amd64.syso"

echo "done. .iconset/ dirs are intermediate (gitignored); commit the .icns and .syso outputs."
