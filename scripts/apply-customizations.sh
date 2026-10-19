#!/usr/bin/env bash
# Overlay this fork's focused changes onto a clean, pinned official checkout.
set -euo pipefail
source_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target="${1:?Usage: apply-customizations.sh OFFICIAL_CHECKOUT}"
expected="f040f28df9fd139e7717af7eaa4c055870ab8339"
test "$(git -C "$target" rev-parse HEAD)" = "$expected" || { echo 'Unexpected upstream version; review the overlay before upgrading.' >&2; exit 1; }
cp "$source_root/server/server.go" "$source_root/server/handlers.go" "$source_root/server/share.go" "$source_root/server/share_test.go" "$target/server/"
cp -R "$source_root/server/web" "$target/server/"
