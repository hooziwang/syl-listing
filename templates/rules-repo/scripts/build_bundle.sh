#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
mkdir -p "$DIST_DIR"

for f in title.yaml bullets.yaml description.yaml search_terms.yaml; do
  test -f "$ROOT_DIR/rules/$f"
done

tar -czf "$DIST_DIR/rules-bundle.tar.gz" -C "$ROOT_DIR/rules" \
  title.yaml bullets.yaml description.yaml search_terms.yaml

echo "bundle: $DIST_DIR/rules-bundle.tar.gz"

