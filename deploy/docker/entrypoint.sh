#!/bin/sh
# CaptchaTunnel server entrypoint.
# Generates a self-signed CA + server certificate on first run so the
# container works standalone (no host-mounted certs required). If certs are
# already mounted/created, they are reused.
set -eu

DOMAIN="${CAPTCHATUNNEL_BASE_DOMAIN:-redy.captchamaster.org}"
TLS_DIR="$(dirname "${CAPTCHATUNNEL_TLS_CERT:-/etc/captchatunnel/tls/server.crt}")"
TLS_KEY="${CAPTCHATUNNEL_TLS_KEY:-/etc/captchatunnel/tls/server.key}"
TLS_CERT="${CAPTCHATUNNEL_TLS_CERT:-/etc/captchatunnel/tls/server.crt}"

mkdir -p "$TLS_DIR"

if [ ! -f "$TLS_CERT" ] || [ ! -f "$TLS_KEY" ]; then
  echo "[entrypoint] generating self-signed control-channel certificate for $DOMAIN"

  # Best-effort public IP so the IP SAN matches how clients dial the server.
  IP=""
  if command -v hostname >/dev/null 2>&1; then
    IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi

  openssl genrsa -out "$TLS_DIR/ca.key" 4096
  openssl req -x509 -new -nodes -key "$TLS_DIR/ca.key" \
    -sha256 -days 3650 \
    -subj "/CN=CaptchaTunnel Root CA" \
    -out "$TLS_DIR/ca.crt"

  openssl genrsa -out "$TLS_KEY" 2048
  openssl req -new -key "$TLS_KEY" \
    -subj "/CN=$DOMAIN" \
    -out /tmp/captchatunnel-server.csr

  {
    printf "subjectAltName = DNS:%s, DNS:*.%s" "$DOMAIN" "$DOMAIN"
    if [ -n "$IP" ]; then
      printf ", IP:%s" "$IP"
    fi
    printf "\nextendedKeyUsage = serverAuth\n"
  } > /tmp/captchatunnel-server.ext

  openssl x509 -req -in /tmp/captchatunnel-server.csr \
    -CA "$TLS_DIR/ca.crt" -CAkey "$TLS_DIR/ca.key" -CAcreateserial \
    -out "$TLS_CERT" -days 825 -sha256 \
    -extfile /tmp/captchatunnel-server.ext

  rm -f /tmp/captchatunnel-server.csr /tmp/captchatunnel-server.ext
  chmod 600 "$TLS_KEY"
  chmod 644 "$TLS_CERT" "$TLS_DIR/ca.crt"
  echo "[entrypoint] certificate ready: $TLS_CERT ($TLS_DIR/ca.crt is the client trust file)"
fi

exec captchatunnel-server