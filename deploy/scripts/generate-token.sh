#!/usr/bin/env bash
# Generate a strong tunnel token. Pipe through: xargs | ./deploy/scripts/generate-token.sh
set -euo pipefail

if command -v openssl >/dev/null 2>&1; then
  openssl rand -hex 32
else
  head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
fi
