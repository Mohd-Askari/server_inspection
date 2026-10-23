#!/usr/bin/env bash
set -euo pipefail

echo "=== Building Debian .deb package for CoreIP Server Inspector ==="
go run ./cmd/package-deb

