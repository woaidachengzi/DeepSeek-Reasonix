#!/usr/bin/env bash
# Native GNU/Linux build only: WebKitGTK and AppImage require target system libs.
set -euo pipefail

if [[ "$(uname -s)" != Linux ]]; then
  echo "Linux Preview requires a native Linux build host; run this script on Linux." >&2
  exit 1
fi
for tool in node pnpm go rustc cargo pkg-config file sha256sum; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing build tool: $tool; install the Linux Preview prerequisites." >&2
    exit 1
  fi
done
node -e 'if (Number(process.versions.node.split(".")[0]) < 24) { console.error("Node 24+ is required"); process.exit(1); }'

task_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
task_frontend_dir="$(cd "$task_script_dir/../../frontend" && pwd)"
task_target="$(rustc --print host-tuple)"
case "$task_target" in
  x86_64-unknown-linux-gnu|aarch64-unknown-linux-gnu) ;;
  *) echo "Unsupported Linux host: $task_target; use native x64/ARM64 GNU Linux." >&2; exit 1 ;;
esac
for library in webkit2gtk-4.1 gtk+-3.0 ayatana-appindicator3-0.1 dbus-1 openssl; do
  if ! pkg-config --exists "$library"; then
    echo "Missing development library: $library; see docs/tauri/LINUX_PREVIEW_ACCEPTANCE.md." >&2
    exit 1
  fi
done
# Keep output paths predictable; accept only the supported package selection.
task_bundles="${1:-deb,appimage}"
if [[ $# -gt 1 ]]; then
  echo "Usage: bash desktop/tauri/scripts/build-linux.sh [deb|appimage|deb,appimage]" >&2
  exit 1
fi
case "$task_bundles" in
  deb|appimage|deb,appimage) ;;
  *) echo "Unsupported bundles: $task_bundles; use deb, appimage, or deb,appimage." >&2; exit 1 ;;
esac

cd "$task_frontend_dir"
pnpm tauri:build -- --target "$task_target" --bundles "$task_bundles"
task_bundle_dir="$task_frontend_dir/../tauri/target/$task_target/release/bundle"
for task_bundle in ${task_bundles//,/ }; do
  case "$task_bundle" in
    deb) task_pattern='*.deb' ;;
    appimage) task_pattern='*.AppImage' ;;
  esac
  task_found=0
  while IFS= read -r -d '' task_artifact; do
    file "$task_artifact"
    sha256sum "$task_artifact"
    task_found=1
  done < <(find "$task_bundle_dir/$task_bundle" -maxdepth 1 -type f -name "$task_pattern" -print0)
  if [[ "$task_found" -eq 0 ]]; then
    echo "Build did not produce a $task_bundle package; inspect the bundler log." >&2
    exit 1
  fi
done
