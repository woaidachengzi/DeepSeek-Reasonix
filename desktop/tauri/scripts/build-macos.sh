#!/bin/bash
# build-macos.sh — Build Reasonix Tauri for macOS
#
# Usage:
#   ./scripts/build-macos.sh              # ad-hoc signing (development)
#   APPLE_SIGNING_IDENTITY="Developer ID Application: ..." ./scripts/build-macos.sh  # release signing
#
# Environment variables:
#   APPLE_SIGNING_IDENTITY  — signing identity (default: ad-hoc "-")
#   APPLE_CERTIFICATE       — base64-encoded .p12 certificate (for CI)
#   APPLE_CERTIFICATE_PASSWORD — certificate password (for CI)
#   APPLE_ID                — Apple ID for notarization (optional)
#   APPLE_PASSWORD          — app-specific password for notarization (optional)
#   APPLE_TEAM_ID           — Apple Developer Team ID (for notarization)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TAURI_DIR="$(dirname "$SCRIPT_DIR")"

cd "$TAURI_DIR"

# --- Configuration ---
SIGNING_IDENTITY="${APPLE_SIGNING_IDENTITY:--}"

echo "=== Reasonix macOS Build ==="
echo "Signing identity: $SIGNING_IDENTITY"

# --- Prerequisites check ---
if ! command -v rustc &> /dev/null; then
    echo "Error: rustc not found. Install Rust first."
    exit 1
fi

if ! command -v node &> /dev/null; then
    echo "Error: node not found. Install Node.js 24+ first."
    exit 1
fi

# Check Node version >= 24
NODE_VERSION=$(node -v | sed 's/v//' | cut -d. -f1)
if [ "$NODE_VERSION" -lt 24 ]; then
    echo "Error: Node.js 24+ required, found $(node -v)"
    exit 1
fi

# --- Build using the existing tauri-build.mjs script ---
echo ""
echo "=== Building Tauri ==="

# Set signing identity for Tauri
export APPLE_SIGNING_IDENTITY="$SIGNING_IDENTITY"

# Run the existing build script which handles sidecar + tauri build
cd "$TAURI_DIR/../frontend"
if command -v pnpm &> /dev/null; then
    pnpm tauri:build
else
    echo "Error: pnpm not found"
    exit 1
fi
cd "$TAURI_DIR"

# --- Output ---
echo ""
echo "=== Build Complete ==="
echo "Artifacts are in: target/release/bundle/"

# --- Notarization (optional) ---
if [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_PASSWORD:-}" ] && [ -n "${APPLE_TEAM_ID:-}" ]; then
    echo ""
    echo "=== Notarizing ==="
    DMG_PATH=$(find target/release/bundle/dmg -name "*.dmg" 2>/dev/null | head -1)
    if [ -n "$DMG_PATH" ]; then
        xcrun notarytool submit "$DMG_PATH" \
            --apple-id "$APPLE_ID" \
            --password "$APPLE_PASSWORD" \
            --team-id "$APPLE_TEAM_ID" \
            --wait
        echo "Notarization complete"
    else
        echo "No DMG found for notarization"
    fi
fi
