#!/usr/bin/env bash
# One-shot Ubuntu 24.04 setup for the CaptchaTunnel server (Docker Compose).
#
# Run as root on a fresh VPS with a public IP and a DNS A record for
# redy.captchamaster.org (and a wildcard *.redy.captchamaster.org).
#
#   sudo ./deploy/scripts/setup-server.sh
set -euo pipefail

DOMAIN="${CAPTCHATUNNEL_BASE_DOMAIN:-redy.captchamaster.org}"
PUBLIC_ADDR="${CAPTCHATUNNEL_PUBLIC_ADDR:-$DOMAIN}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$ROOT_DIR"

echo "==> Updating packages"
apt-get update -y
apt-get upgrade -y
apt-get install -y curl ca-certificates openssl

# --- Docker ----------------------------------------------------------------
if ! command -v docker >/dev/null 2>&1; then
  echo "==> Installing Docker"
  curl -fsSL https://get.docker.com | sh
fi

# --- Generate token --------------------------------------------------------
if [ ! -f .env ]; then
  echo "==> Writing .env"
  cp .env.example .env
  TOKEN="$(bash deploy/scripts/generate-token.sh)"
  sed -i "s|^CAPTCHATUNNEL_TOKEN=.*|CAPTCHATUNNEL_TOKEN=${TOKEN}|" .env
  sed -i "s|^CAPTCHATUNNEL_BASE_DOMAIN=.*|CAPTCHATUNNEL_BASE_DOMAIN=${DOMAIN}|" .env
  sed -i "s|^CAPTCHATUNNEL_PUBLIC_ADDR=.*|CAPTCHATUNNEL_PUBLIC_ADDR=${PUBLIC_ADDR}|" .env
  chmod 600 .env
  echo "    Token written to .env (keep it secret)."
fi

# --- Control-channel TLS certs ---------------------------------------------
if [ ! -f tls/server.crt ]; then
  echo "==> Generating self-signed control-channel certificate"
  bash deploy/scripts/gen-server-cert.sh "$DOMAIN" tls
fi

# --- Public wildcard certificate (optional, prompts) -----------------------
if [ ! -f tls/fullchain.pem ]; then
  echo
  echo "NOTE: tls/fullchain.pem not found. The public 443 TLS cert (wildcard) is"
  echo "required for https://<subdomain>.$DOMAIN to work. Options:"
  echo "  1. Run: bash deploy/scripts/obtain-wildcard-cert.sh $DOMAIN your@email"
  echo "  2. Or copy existing fullchain.pem/privkey.pem into ./tls"
  echo
fi

# --- Firewall --------------------------------------------------------------
if command -v ufw >/dev/null 2>&1; then
  echo "==> Configuring firewall"
  ufw allow OpenSSH
  ufw allow 80/tcp
  ufw allow 443/tcp
  ufw allow 4443/tcp
  ufw allow 10000:10100/tcp
  ufw --force enable
fi

# --- Launch ----------------------------------------------------------------
echo "==> Building and starting containers"
docker compose up -d --build

echo
echo "Done. Verify with:"
echo "  docker compose ps"
echo "  docker compose logs -f server"
echo
echo "Client token: $(grep '^CAPTCHATUNNEL_TOKEN=' .env | cut -d= -f2)"
echo "Test:         captchatunnel http 3000 --server=$DOMAIN:4443 --tls-ca=tls/ca.crt --token=<token>"
