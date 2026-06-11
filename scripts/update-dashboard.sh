#!/usr/bin/env bash
# Re-vendor the bundled claude-dashboard status line from upstream.
# Usage: scripts/update-dashboard.sh <tag>   (e.g. v1.29.0)
set -euo pipefail
tag="${1:?usage: update-dashboard.sh <tag>}"
base="https://raw.githubusercontent.com/uppinote20/claude-dashboard/${tag}"
curl -fsSL "${base}/dist/index.js" -o internal/dashboard/dashboard.mjs
curl -fsSL "${base}/LICENSE"       -o internal/dashboard/LICENSE

# Strip upstream's embedded Google OAuth client id/secret (it powers their
# Gemini-usage widget). fleet must not redistribute a third party's secret, and
# the core status line (model/context/cost/Claude rate limits) works without it.
sed -i.bak -E 's/(OAUTH_CLIENT_ID = )"[^"]*"/\1""/; s/(OAUTH_CLIENT_SECRET = )"[^"]*"/\1""/' internal/dashboard/dashboard.mjs
rm -f internal/dashboard/dashboard.mjs.bak
if grep -qE 'GOCSPX-|apps\.googleusercontent\.com' internal/dashboard/dashboard.mjs; then
  echo "ERROR: a Google OAuth secret survived the strip — refusing to vendor" >&2
  exit 1
fi

echo "Vendored claude-dashboard ${tag} (Gemini OAuth creds stripped) → internal/dashboard/ (rebuild fleet to embed)"
