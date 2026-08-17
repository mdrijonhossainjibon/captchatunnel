# syntax=docker/dockerfile:1
#
# Build:  docker build -f deploy/docker/Dockerfile -t captchatunnel-server .
# The image contains only the statically-linked server binary.

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
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/captchatunnel-server /usr/local/bin/captchatunnel-server
USER app

# 4443 = control channel (TLS), 8080 = HTTP ingress (nginx upstream),
# 10000-10100 = per-tunnel TCP ports.
EXPOSE 4443 8080 10000-10100

ENTRYPOINT ["captchatunnel-server"]
