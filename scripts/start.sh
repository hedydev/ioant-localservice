#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"
mkdir -p bin
go build -o bin/localservice ./cmd/localservice
exec ./bin/localservice "$@"
