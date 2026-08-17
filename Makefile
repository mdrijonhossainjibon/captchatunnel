SHELL := /bin/bash

BINARY_CLIENT  := captchatunnel
BINARY_SERVER  := captchatunnel-server
PKG            := github.com/captchamaster/captchatunnel
VERSION        ?= 1.0.0

LDFLAGS := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION)

.PHONY: all build build-client build-server build-linux build-linux-amd64 build-linux-arm64 tidy test fmt vet clean

all: build

build: build-client build-server

build-client:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_CLIENT) ./cmd/client

build-server:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_SERVER) ./cmd/server

# Static binaries for the VPS (Ubuntu 24.04, glibc/musl agnostic).
build-linux: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/captchatunnel-linux-amd64 ./cmd/client
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/captchatunnel-server-linux-amd64 ./cmd/server

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/captchatunnel-linux-arm64 ./cmd/client
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/captchatunnel-server-linux-arm64 ./cmd/server

tidy:
	go mod tidy

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin dist
