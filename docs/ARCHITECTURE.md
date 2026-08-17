# Architecture

CaptchaTunnel is a client-initiated, multiplexed reverse tunnel. The client
always dials **out** to the server, so it works behind NAT and firewalls
without any inbound port forwarding.

## Data plane overview

```
                        VPS (public)
   Browser                                     User machine (NAT)
     │                                            │
     │  HTTPS 443                                   │
     ▼                                            │
  ┌─────────┐   HTTP 8080    ┌───────────────┐   │
  │  Nginx  │ ──────────────▶ │ tunnel server │   │
  └─────────┘                 │  (Go)         │   │
     wildcard TLS             │               │   │
                              │  ┌─────────┐  │   │
                              │  │ registry│  │   │
                              │  └─────────┘  │   │
                              └──────┬────────┘   │
                                     │ TLS 4443    │
                                     │ + yamux     │
                                     ▼            │
                              ┌───────────────┐   │
                              │ tunnel client │   │
                              └──────┬────────┘   │
                                     ▼            │
                              localhost:3000 ◀────┘
```

1. The client opens one outbound TLS connection to the server control port
   (`4443`) and wraps it in a `yamux` multiplexing session.
2. Over the first yamux stream it performs a token handshake and registers a
   tunnel (protocol, subdomain, target).
3. For HTTP(S), Nginx terminates the public TLS and forwards the request to the
   server's HTTP ingress (`8080`), preserving the `Host` header.
4. The server parses only the `Host` header to find the subdomain, looks up the
   tunnel, and opens a **new yamux stream** on the client's session. It then
   relays raw bytes between the public connection and that stream.
5. The client accepts the stream, dials the local target, and relays raw bytes.
   The local app's response flows back the same way.

Because the relay is byte-transparent, WebSocket, SSE, streaming, chunked
bodies, and arbitrary TCP all work without protocol-specific parsing.

## Control channel

The control channel is a TLS connection carrying a yamux session. The first
stream is the **control stream**; subsequent streams are **data streams**
opened by the server for each public connection.

### Handshake (replay-protected)

```
client                                 server
  │  hello {version, clientNonce} ─────▶│
  │  ◀───────── challenge {serverNonce} │
  │  register {proto, subdomain,        │
  │            target, auth} ──────────▶│
  │  ◀────────── registered {url, ...}  │
```

`auth = HMAC-SHA256(token, serverNonce : clientNonce)`. The token is never
transmitted; only the server and a legitimate client can compute the proof.
The nonce is fresh per handshake, so a captured proof cannot be replayed.

### Heartbeat and liveness

- The client sends a `ping` every 5 s; the server answers `pong`.
- The server updates the tunnel's `lastSeen` on every ping and evicts tunnels
  that miss heartbeats for 15 s.
- Both sides enable yamux transport keep-alive to detect dead TCP connections
  at the transport layer.
- The client monitors `pong` replies; if none arrive within the timeout it
  closes the session and reconnects.

## Reconnect and reclaim

- The client reconnects with exponential backoff + jitter (1 s → 30 s).
- It re-requests its previously assigned subdomain, so the public URL is stable
  across reconnects and client restarts.
- The registry enforces **one tunnel per token**: a new registration evicts the
  stale tunnel with the same token and atomically reuses its name/port.
- A taken *auto-assigned* subdomain triggers a fresh random subdomain; a taken
  *explicitly requested* subdomain is reported as a permanent error.

## Tunnel types

### HTTP / HTTPS

- Nginx owns port 80/443 and the wildcard certificate.
- The server's HTTP ingress (`8080`) reads only the header block to extract the
  `Host` header, then relays the raw request over yamux.
- The Nginx config sends `Connection: close` upstream for ordinary requests
  (so the transparent relay knows when a response ends) and `upgrade` for
  WebSocket/HTTP upgrade.

### TCP

- The server allocates a port from a configured range (`10000-10100`) and binds
  a dedicated listener per tunnel.
- Each inbound connection maps to a fresh yamux stream and is relayed raw.
- Public URL is `tcp://<public-addr>:<port>`.

## Concurrency model

- Every public connection is one goroutine pair (two `io.Copy` directions) and
  one yamux stream; no per-connection buffering beyond socket buffers.
- A single yamux session carries all connections for a tunnel, keeping
  connection setup cheap (no per-request TLS handshake to the client).
- The registry is guarded by a single `sync.RWMutex`; per-tunnel state uses
  atomics where appropriate.

## Request validation & isolation

- Unknown `Host` values and unroutable subdomains get a minimal `404`.
- Header blocks are bounded (64 KiB) to prevent unbounded buffering.
- A client can only ever receive traffic for the subdomain/port it was
  assigned; there is no cross-tunnel addressing.

## Known limitation

`hashicorp/yamux` does not expose half-close (TCP FIN) per stream. The relay
therefore closes a stream fully when either direction reaches EOF. This is
correct for HTTP/1.1-with-close, WebSocket, SSE, SSH, and Minecraft; a protocol
that strictly requires independent write-half closure would need a multiplexer
with half-close support (e.g. `xtaci/smux`) — a drop-in swap at the two
`yamux` call sites.
