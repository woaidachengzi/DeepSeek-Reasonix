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

(
  cd "$bridge_root"
  GOCACHE="$work_root/go-cache" GOPROXY=off go build -o "$bridge_bin" ./cmd/reasonix-desktop-bridge
)

sidecar_dir="$legacy_root/desktop/tauri/binaries"
mkdir -p "$sidecar_dir"
cp "$bridge_bin" "$sidecar_dir/reasonix-desktop-bridge-$host_triple"

run_host_integration() {
  local test_name="$1"
  local profile_root="$work_root/profile-$test_name"
  mkdir -p "$profile_root"
  (
    cd "$legacy_root/desktop/tauri"
    REASONIX_HOME="$profile_root/home" \
      REASONIX_STATE_HOME="$profile_root/state" \
      REASONIX_TAURI_BRIDGE_TEST_BIN="$bridge_bin" \
      CARGO_TARGET_DIR="$target_root" \
      cargo test --offline --manifest-path "$legacy_root/desktop/tauri/Cargo.toml" "$test_name"
  )
}

run_host_integration supervisor_starts_and_stops_a_real_bridge_when_provided
run_host_integration session_title_survives_a_sidecar_restart
run_host_integration deleting_a_session_removes_its_artifacts_and_keeps_other_sessions

(
  cd "$legacy_root/desktop/tauri"
  CARGO_TARGET_DIR="$target_root" \
    cargo build --offline --manifest-path "$legacy_root/desktop/tauri/Cargo.toml"
)

host_bin="$target_root/debug/reasonix-tauri"
printf 'legacy host source: %s\n' "$legacy_rev"
printf 'current bridge source: %s\n' "$current_rev"
printf 'candidate host binary: %s\n' "$host_bin"
shasum -a 256 "$host_bin"
printf 'isolated artifacts and profiles: %s\n' "$work_root"
