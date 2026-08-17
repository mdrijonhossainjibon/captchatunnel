package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the client-side tunnel settings.
type Config struct {
	Proto      string // "http" | "tcp"
	Target     string // dialable local address, e.g. 127.0.0.1:3000
	Display    string // human-friendly target, e.g. localhost:3000
	Subdomain  string // empty => auto-generate
	Token      string
	Region     string

	ServerAddr    string // e.g. redy.captchamaster.org:4443
	TLSCA         string // path to CA certificate to pin
	TLSSkipVerify bool
	TLSServerName string // override TLS SNI/verification name

	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

// ResolveTarget normalizes a "PORT" or "HOST:PORT" target into a dialable
// address. A bare port binds to loopback.
func ResolveTarget(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("missing target (use PORT or HOST:PORT)")
	}
	if isDigits(s) {
		p, err := strconv.Atoi(s)
		if err != nil || p < 1 || p > 65535 {
			return "", fmt.Errorf("invalid port %q", raw)
		}
		return net.JoinHostPort("127.0.0.1", s), nil
	}
	host, port, err := net.SplitHostPort(s)
	if err == nil {
		if !isDigits(port) {
			return "", fmt.Errorf("invalid target %q", raw)
		}
		p, _ := strconv.Atoi(port)
		if p < 1 || p > 65535 {
			return "", fmt.Errorf("invalid port %q", raw)
		}
		return net.JoinHostPort(host, port), nil
	}
	return "", fmt.Errorf("invalid target %q (want PORT or HOST:PORT)", raw)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// TLSConfig builds the client TLS configuration for the control channel.
//
// If no CA is configured the client trusts the server certificate on-the-fly
// (auto-trust, like SSH/cloudflared), so a fresh `captchatunnel 3000` works
// with no --tls-ca / config file. For strict verification, provide --tls-ca.
func (c *Config) TLSConfig() (*tls.Config, error) {
	conf := &tls.Config{MinVersion: tls.VersionTLS12}

	if c.TLSSkipVerify || c.TLSCA == "" {
		// Auto-trust (skip verification) when the user did not pin a CA.
		conf.InsecureSkipVerify = true
	}
	if c.TLSCA != "" {
		pem, err := os.ReadFile(c.TLSCA)
		if err != nil {
			return nil, fmt.Errorf("reading CA %s: %w", c.TLSCA, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no valid certificates in %s", c.TLSCA)
		}
		conf.RootCAs = pool
	}
	if c.TLSServerName != "" {
		conf.ServerName = c.TLSServerName
	} else if host, _, err := net.SplitHostPort(c.ServerAddr); err == nil {
		conf.ServerName = host
	}
	return conf, nil
}
