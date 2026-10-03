#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
legacy_rev="50c1b9bc6"
current_rev="$(git -C "$repo_root" rev-parse HEAD)"
work_root="$(mktemp -d "${TMPDIR:-/private/tmp}/reasonix-tauri-compat.XXXXXX")"
legacy_root="$work_root/legacy"
bridge_root="$work_root/current"
bridge_bin="$work_root/reasonix-desktop-bridge"
target_root="$work_root/target"
host_triple="$(rustc -vV | sed -n 's/^host: //p')"

git -C "$repo_root" cat-file -e "${legacy_rev}^{commit}"
mkdir -p "$legacy_root" "$bridge_root"
git -C "$repo_root" archive "$legacy_rev" | tar -x -C "$legacy_root"
git -C "$repo_root" archive "$current_rev" | tar -x -C "$bridge_root"
# The compatibility run must exercise the bridge that is actually in the
# worktree, including staged and unstaged edits. Keep the source archive
# reproducible from HEAD, then overlay its complete tracked diff and any
# non-ignored untracked source files into the isolated build copy.
git -C "$repo_root" diff --binary HEAD -- | git -C "$bridge_root" apply --binary
git -C "$repo_root" ls-files --others --exclude-standard -z \
  | tar --null -T - -cf - \
  | tar -xf - -C "$bridge_root"

(
  cd "$bridge_root"
  GOCACHE="$work_root/go-cache" GOPROXY=off go build -o "$bridge_bin" ./cmd/reasonix-desktop-bridge
)

sidecar_dir="$legacy_root/desktop/tauri/binaries"
mkdir -p "$sidecar_dir"
cp "$bridge_bin" "$sidecar_dir/reasonix-desktop-bridge-$host_triple"

run_host_compatibility_suite() {
  local profile_root="$work_root/profile-compatibility-suite"
  mkdir -p "$profile_root"
  (
    cd "$legacy_root/desktop/tauri"
    REASONIX_HOME="$profile_root/home" \
      REASONIX_STATE_HOME="$profile_root/state" \
      REASONIX_TAURI_BRIDGE_TEST_BIN="$bridge_bin" \
      CARGO_TARGET_DIR="$target_root" \
      cargo test --offline --manifest-path "$legacy_root/desktop/tauri/Cargo.toml" -- --test-threads=1
  )
}

run_host_compatibility_suite

(
  cd "$legacy_root/desktop/tauri"
  CARGO_TARGET_DIR="$target_root" \
    cargo build --offline --manifest-path "$legacy_root/desktop/tauri/Cargo.toml"
)

host_bin="$target_root/debug/reasonix-tauri"
printf 'legacy host source: %s\n' "$legacy_rev"
printf 'current bridge HEAD: %s\n' "$current_rev"
printf 'current bridge tracked diff: '
git -C "$repo_root" diff --binary HEAD -- | shasum -a 256
printf 'current bridge binary: %s\n' "$bridge_bin"
shasum -a 256 "$bridge_bin"
printf 'candidate host binary: %s\n' "$host_bin"
shasum -a 256 "$host_bin"
printf 'isolated artifacts and profiles: %s\n' "$work_root"
