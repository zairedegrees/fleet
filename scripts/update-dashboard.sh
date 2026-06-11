#!/usr/bin/env bash
# Re-vendor the bundled claude-dashboard status line from upstream.
# Usage: scripts/update-dashboard.sh <tag>   (e.g. v1.29.0)
set -euo pipefail
tag="${1:?usage: update-dashboard.sh <tag>}"
base="https://raw.githubusercontent.com/uppinote20/claude-dashboard/${tag}"
curl -fsSL "${base}/dist/index.js" -o internal/dashboard/dashboard.mjs
curl -fsSL "${base}/LICENSE"       -o internal/dashboard/LICENSE
echo "Vendored claude-dashboard ${tag} → internal/dashboard/ (rebuild fleet to embed)"
