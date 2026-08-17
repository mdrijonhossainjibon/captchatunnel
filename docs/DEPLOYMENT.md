# Deployment Guide

This guide deploys the tunnel **server** on a VPS and builds the **client**
binary for users.

## Prerequisites

- Ubuntu 24.04 VPS with a public IP.
- A domain, e.g. `redy.captchamaster.org`.
- DNS records pointing to the VPS:
  - `A redy.captchamaster.org -> <VPS-IP>`
  - `A *.redy.captchamaster.org -> <VPS-IP>` (or a DNS provider that supports
    wildcard records; the wildcard cert uses DNS-01 anyway).
- Docker + Docker Compose v2 (installed by the setup script if missing).

## 1. Get the code

```bash
git clone <your-repo-url> captchatunnel && cd captchatunnel
```

## 2. Generate secrets and certificates

```bash
cp .env.example .env
chmod 600 .env

# Tunnel token
TOKEN=$(bash deploy/scripts/generate-token.sh)
sed -i "s|^CAPTCHATUNNEL_TOKEN=.*|CAPTCHATUNNEL_TOKEN=${TOKEN}|" .env

# Control-channel certificate (self-signed CA + server cert)
bash deploy/scripts/gen-server-cert.sh redy.captchamaster.org tls
```

The `ca.crt` in `tls/` is what clients pass to `--tls-ca`.

## 3. Public wildcard certificate (Let's Encrypt)

Wildcard certificates require the **DNS-01** challenge.

```bash
bash deploy/scripts/obtain-wildcard-cert.sh redy.captchamaster.org you@example.com
```

The script copies `fullchain.pem` and `privkey.pem` into `tls/`. For
automation use a DNS plugin (Cloudflare example is in the script).

## 4. Launch with Docker Compose

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f server
```

Ports published:

| Port          | Purpose                                  |
| ------------- | ---------------------------------------- |
| 80, 443       | Public HTTP/HTTPS (Nginx)                |
| 4443          | Control channel (TLS, clients connect)   |
| 10000-10100   | Public TCP tunnels                       |

The server's HTTP ingress (`8080`) is only reachable from Nginx on the internal
Docker network; it is not published.

## 5. Firewall (UFW)

```bash
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 4443/tcp
ufw allow 10000:10100/tcp
ufw --force enable
```

## 6. Build the client binary

On any machine with Go 1.22+:

```bash
make build-linux-amd64          # dist/captchatunnel-linux-amd64
make build-linux-arm64          # dist/captchatunnel-linux-arm64
```

Distribute the single binary to users. No other files are required (the CA can
be embedded or distributed alongside; see "Client trust").

## 7. Client usage

```bash
captchatunnel http 3000 \
  --server=redy.captchamaster.org:4443 \
  --token=<TOKEN> \
  --tls-ca=./ca.crt

captchatunnel tcp 22 --server=... --token=... --tls-ca=./ca.crt
```

Or bake defaults into `~/.captchatunnel/config.json`:

```json
{ "token": "<TOKEN>" }
```

and set `CAPTCHATUNNEL_SERVER=redy.captchamaster.org:4443`.

## Host install (alternative to Docker)

If you prefer a bare binary + systemd instead of containers:

```bash
# build the static server binary
make build-server

sudo useradd --system --home /etc/captchatunnel --shell /usr/sbin/nologin captchatunnel
sudo install -m 0755 bin/captchatunnel-server /usr/local/bin/captchatunnel-server

sudo mkdir -p /etc/captchatunnel/tls
sudo cp tls/server.crt tls/server.key /etc/captchatunnel/tls/
sudo cp .env /etc/captchatunnel/captchatunnel.env
sudo chmod 600 /etc/captchatunnel/captchatunnel.env

# adjust CAPTCHATUNNEL_TLS_CERT/KEY paths to /etc/captchatunnel/tls/...
sudo cp deploy/systemd/captchatunnel-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now captchatunnel-server
```

For a host install, Nginx runs on the host too; change `proxy_pass` in
`deploy/nginx/tunnel.conf` from `server:8080` to `127.0.0.1:8080`.

## TLS overview

| Layer              | Cert                                 | Terminated by |
| ------------------ | ------------------------------------ | ------------- |
| Public 443         | `fullchain.pem` / `privkey.pem`      | Nginx         |
| Control 4443       | `server.crt` / `server.key`          | Go server     |
| Client trust       | `ca.crt` (via `--tls-ca`)            | Go client     |

The control channel enforces TLS 1.3 where available (1.2 fallback). For a
fully public control channel you can also sign `server.crt` with a public CA
and drop `--tls-ca`.

## Verification checklist

```bash
# server is up
docker compose ps

# open a test tunnel from your laptop
captchatunnel http 3000 --server=<VPS>:4443 --token=<TOKEN> --tls-ca=tls/ca.crt

# in another terminal
curl -v https://<printed-subdomain>.redy.captchamaster.org

# TCP test
captchatunnel tcp 8080 --server=<VPS>:4443 --token=<TOKEN> --tls-ca=tls/ca.crt
nc <VPS> 10000
```

## Updating

```bash
git pull
docker compose up -d --build
```

## Operations

- **Logs:** `docker compose logs -f server`
- **Restart:** `docker compose restart`
- **Tunnels are ephemeral** (no database). Dead tunnels are evicted by
  heartbeat timeout (default 15 s).
- **Scale:** for thousands of tunnels, raise `LimitNOFILE`/container ulimits
  and widen `CAPTCHATUNNEL_TCP_PORT_MAX`. Memory stays low because tunnels are
  goroutine-based.
