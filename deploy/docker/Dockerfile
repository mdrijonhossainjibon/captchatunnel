# syntax=docker/dockerfile:1
#
# Build:  docker build -f deploy/docker/Dockerfile -t captchatunnel-server .
# The image is self-contained: the entrypoint generates a self-signed TLS
# certificate on first run, so it deploys cleanly as a Coolify resource or a
# bare `docker run`.

FROM golang:1.22-alpine AS build
WORKDIR /src

# Download dependencies first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=1.0.0
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X github.com/captchamaster/captchatunnel/internal/version.Version=${VERSION}" \
    -o /out/captchatunnel-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata openssl \
    && addgroup -S app \
    && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/captchatunnel-server /usr/local/bin/captchatunnel-server
COPY deploy/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# 4443 = control channel (TLS), 8080 = HTTP ingress (bound publicly on 2086
# behind Cloudflare), 10000-10100 = per-tunnel TCP ports.
EXPOSE 4443 8080 2086 10000-10100

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]