#!/usr/bin/env bash
# Generate a self-signed CA + server certificate for the control channel.
#
# Usage:
#   ./deploy/scripts/gen-server-cert.sh [domain] [output-dir]
#
# The client then trusts the CA with: --tls-ca=./tls/ca.crt
set -euo pipefail

DOMAIN="${1:-redy.captchamaster.org}"
OUT="${2:-tls}"
mkdir -p "$OUT"

echo "Generating CA + server certificate for $DOMAIN in $OUT/"

# 1. Certificate authority.
openssl genrsa -out "$OUT/ca.key" 4096
openssl req -x509 -new -nodes -key "$OUT/ca.key" \
  -sha256 -days 3650 \
  -subj "/CN=CaptchaTunnel Root CA" \
  -out "$OUT/ca.crt"

# 2. Server key + CSR.
openssl genrsa -out "$OUT/server.key" 2048
openssl req -new -key "$OUT/server.key" \
  -subj "/CN=$DOMAIN" \
  -out "$OUT/server.csr"

# 3. Sign with SANs for the control domain and its wildcard.
cat > "$OUT/server.ext" <<EOF
subjectAltName = DNS:$DOMAIN, DNS:*.$DOMAIN
extendedKeyUsage = serverAuth
EOF
openssl x509 -req -in "$OUT/server.csr" \
  -CA "$OUT/ca.crt" -CAkey "$OUT/ca.key" -CAcreateserial \
  -out "$OUT/server.crt" -days 825 -sha256 \
  -extfile "$OUT/server.ext"

rm -f "$OUT/server.csr" "$OUT/server.ext"
chmod 600 "$OUT"/*.key

echo
echo "Generated:"
ls -l "$OUT"
echo
echo "Server uses:   $OUT/server.crt / $OUT/server.key"
echo "Client trusts: --tls-ca=$OUT/ca.crt"
