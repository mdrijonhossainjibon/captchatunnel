# CaptchaTunnel

A self-hosted tunnel platform in the style of Cloudflare Tunnel / Ngrok. A user
installs a single CLI binary and runs:

```bash
captchatunnel http 3000
```

and gets a public HTTPS URL:

```text
✓ Connected

Public URL:
https://ab29fd.redy.captchamaster.org

Forwarding:
http://localhost:3000

Status:
Online
```

The CLI opens an encrypted tunnel from the user's machine to your VPS, and
Nginx routes `*.redy.captchamaster.org` through the tunnel to the user's local
service.

- **No frontend, no dashboard, no database, no user registration.**
- One shared tunnel token authenticates every client.
- HTTP, HTTPS, TCP, WebSocket, SSE and raw TCP relays all work.

## Components

| Component            | Description                                                              |
| -------------------- | ------------------------------------------------------------------------ |
| Tunnel server        | Go daemon on the VPS (`cmd/server`). Terminates control connections, holds the tunnel registry, forwards traffic. |
| Tunnel client        | Single CLI binary (`cmd/client`, `captchatunnel`). Connects out, relays traffic to localhost. |
| Reverse proxy        | Nginx terminates public TLS and forwards `*.domain` to the server.       |
| TLS layer            | Public wildcard cert (Let's Encrypt) + control-channel cert (self-signed CA). |

## Architecture

```
  Browser ──HTTPS──> Nginx :443 ──HTTP──> server :8080 ──yamux──> client ──> localhost:3000
                     (wildcard TLS)         (route by Host)      (single mux)    (local app)

  TCP peer ──> server :10000-10100 ──yamux──> client ──> localhost:22
```

The client dials **out** to the server (works behind NAT) over TLS, then
multiplexes all public connections over a single `yamux` session. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Project layout

```
captchatunnel/
├── cmd/
│   ├── server/            server entrypoint
│   └── client/            CLI entrypoint (captchatunnel)
├── internal/
│   ├── protocol/          control wire messages + framing
│   ├── auth/              token proof (HMAC challenge) + replay protection
│   ├── tunnel/            tunnel types + subdomain helpers
│   ├── relay/             bidirectional byte pipe
│   ├── server/            registry, control, HTTP/TCP ingress
│   └── client/            connect, heartbeat, reconnect, relay, UI
├── deploy/
│   ├── docker/            Dockerfile
│   ├── nginx/             reverse proxy config
│   ├── systemd/           host-install unit
│   └── scripts/           token/cert/setup helpers
├── tls/                   generated certs (gitignored)
├── docker-compose.yml
└── docs/                  deployment + architecture guides
```

## Quick start (client)

```bash
# build
make build-client

# expose a local web app
./bin/captchatunnel http 3000 --server=redy.captchamaster.org:4443 \
  --token=YOUR_TOKEN --tls-ca=tls/ca.crt

# expose SSH
./bin/captchatunnel tcp 22 --server=redy.captchamaster.org:4443 \
  --token=YOUR_TOKEN --tls-ca=tls/ca.crt
```

Options: `--subdomain=NAME`, `--token=SECRET`, `--region=NAME`,
`--server=ADDR`, `--tls-ca=FILE`, `--tls-skip-verify`, `--tls-server-name=NAME`.

Token resolution order: `--token` → `CAPTCHATUNNEL_TOKEN` env →
`~/.captchatunnel/config.json` (`{"token":"..."}`).

## Server deployment

Full steps: [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md). Short version on Ubuntu 24.04:

```bash
git clone <this-repo> && cd captchatunnel
cp .env.example .env
# put your token in .env
bash deploy/scripts/gen-server-cert.sh redy.captchamaster.org tls
bash deploy/scripts/obtain-wildcard-cert.sh redy.captchamaster.org you@example.com
docker compose up -d --build
```

Or run the one-shot installer: `sudo bash deploy/scripts/setup-server.sh`.

## Building

Requires Go 1.22+.

```bash
make build            # build both binaries into bin/
make build-linux      # cross-compile static Linux binaries into dist/
```

## Security

- TLS 1.3 (1.2 fallback) for both the public edge and the control channel.
- Token authentication via an HMAC challenge-response (the token is never sent).
- Fresh nonce per handshake → replay protection.
- Constant-time token comparison.
- One tunnel per token; a reconnect reclaims the same subdomain.
- Per-tunnel isolation: each tunnel only forwards its own assigned name/port.
