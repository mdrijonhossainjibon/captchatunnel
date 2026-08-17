#!/usr/bin/env bash
# Obtain a public wildcard certificate for *.redy.captchamaster.org from Let's
# Encrypt using the DNS-01 challenge (required for wildcard names).
#
# Pick one DNS plugin below, then run as root. Outputs fullchain.pem and
# privkey.pem into ./tls for the Nginx container.
set -euo pipefail

DOMAIN="${1:-redy.captchamaster.org}"
EMAIL="${2:-admin@${DOMAIN}}"
OUT="tls"
mkdir -p "$OUT"

if ! command -v certbot >/dev/null 2>&1; then
  echo "Installing certbot..."
  apt-get update -y
  apt-get install -y certbot
fi

# --- Cloudflare (recommended) ----------------------------------------------
# 1. Create an API token with "Zone.Zone Read" + "Zone.DNS Edit".
# 2. Write it to ~/.secrets/cloudflare.ini:
#      dns_cloudflare_api_token = YOUR_TOKEN
# 3. Uncomment:
#
# apt-get install -y python3-certbot-dns-cloudflare
# certbot certonly \
#   --dns-cloudflare \
#   --dns-cloudflare-credentials ~/.secrets/cloudflare.ini \
#   -d "$DOMAIN" -d "*.$DOMAIN" \
#   --email "$EMAIL" --agree-tos --non-interactive

# --- Generic DNS plugin -----------------------------------------------------
# Install the matching plugin (e.g. python3-certbot-dns-route53 for AWS) and
# call certbot with the appropriate --dns-<provider> flags.

# --- Manual (no plugin) -----------------------------------------------------
echo "No DNS plugin configured. Running interactive DNS-01:"
echo "Follow the prompts to create a TXT record _acme-challenge.$DOMAIN."
certbot certonly \
  --manual \
  --preferred-challenges dns \
  -d "$DOMAIN" -d "*.$DOMAIN" \
  --email "$EMAIL" --agree-tos --no-eff-email

# Symlink/copy the issued certificate into ./tls for Nginx.
CERT_DIR="/etc/letsencrypt/live/$DOMAIN"
if [ -d "$CERT_DIR" ]; then
  cp -L "$CERT_DIR/fullchain.pem" "$OUT/fullchain.pem"
  cp -L "$CERT_DIR/privkey.pem"   "$OUT/privkey.pem"
  chmod 600 "$OUT/privkey.pem"
  echo "Copied fullchain.pem + privkey.pem to $OUT/"
else
  echo "Certificate directory $CERT_DIR not found; copy your certs into $OUT manually."
fi
