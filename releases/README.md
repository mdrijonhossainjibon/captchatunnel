# CaptchaTunnel client binaries

Download the right one for your machine.

| File | Platform |
| --- | --- |
| `captchatunnel-linux-amd64` | Linux 64-bit (Intel/AMD) — most servers, Colab |
| `captchatunnel-linux-arm64` | Linux ARM (Raspberry Pi, ARM servers) |
| `captchatunnel-windows-amd64.exe` | Windows 64-bit |
| `ca-coolify.crt` | Control-channel CA (needed by `--tls-ca` / config) |

## Direct download (raw URL)

Replace `<VERSION>` with the git tag/branch, e.g. `main`:

```
https://raw.githubusercontent.com/mdrijonhossainjibon/captchatunnel/main/releases/captchatunnel-linux-amd64
https://raw.githubusercontent.com/mdrijonhossainjibon/captchatunnel/main/releases/ca-coolify.crt
```

## Linux / Colab quick start

```bash
wget -O captchatunnel https://raw.githubusercontent.com/mdrijonhossainjibon/captchatunnel/main/releases/captchatunnel-linux-amd64
wget -O ca-coolify.crt   https://raw.githubusercontent.com/mdrijonhossainjibon/captchatunnel/main/releases/ca-coolify.crt
chmod +x captchatunnel

mkdir -p ~/.captchatunnel
cat > ~/.captchatunnel/config.json <<'EOF'
{
  "server": "148.113.59.83:4443",
  "tls_ca": "./ca-coolify.crt",
  "tls_server_name": "redy.captchamaster.org"
}
EOF

./captchatunnel 3000     # -> https://<random>.redy.captchamaster.org
```

> `ca-coolify.crt` changes on every Coolify redeploy of the server. If the client
> reports `certificate signed by unknown authority`, re-download this CA file.
