package server

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all server settings. Values are read from environment
// variables with sensible defaults so the same binary works in Docker,
// under systemd, and in a bare process.
type Config struct {
	Token   string
	BaseDomain string
	PublicAddr string

	ControlAddr string
	HTTPAddr    string
	TCPAddr     string
	TCPPortMin  int
	TCPPortMax  int

	TLSCert string
	TLSKey  string

	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

// LoadConfig builds a Config from the environment.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Token:             getenv("CAPTCHATUNNEL_TOKEN", ""),
		BaseDomain:        getenv("CAPTCHATUNNEL_BASE_DOMAIN", "redy.captchamaster.org"),
		PublicAddr:        getenv("CAPTCHATUNNEL_PUBLIC_ADDR", "redy.captchamaster.org"),
		ControlAddr:       getenv("CAPTCHATUNNEL_CONTROL_ADDR", "0.0.0.0:4443"),
		HTTPAddr:          getenv("CAPTCHATUNNEL_HTTP_ADDR", "0.0.0.0:8080"),
		TCPAddr:           getenv("CAPTCHATUNNEL_TCP_ADDR", "0.0.0.0"),
		TCPPortMin:        getenvInt("CAPTCHATUNNEL_TCP_PORT_MIN", 10000),
		TCPPortMax:        getenvInt("CAPTCHATUNNEL_TCP_PORT_MAX", 10100),
		TLSCert:           getenv("CAPTCHATUNNEL_TLS_CERT", ""),
		TLSKey:            getenv("CAPTCHATUNNEL_TLS_KEY", ""),
		HeartbeatInterval: getenvDuration("CAPTCHATUNNEL_HEARTBEAT_INTERVAL", 5*time.Second),
		HeartbeatTimeout:  getenvDuration("CAPTCHATUNNEL_HEARTBEAT_TIMEOUT", 15*time.Second),
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("CAPTCHATUNNEL_TOKEN is required")
	}
	if cfg.TLSCert == "" || cfg.TLSKey == "" {
		return nil, fmt.Errorf("CAPTCHATUNNEL_TLS_CERT and CAPTCHATUNNEL_TLS_KEY are required")
	}
	if cfg.TCPPortMax < cfg.TCPPortMin {
		return nil, fmt.Errorf("TCP port range is inverted")
	}
	cfg.BaseDomain = strings.ToLower(strings.TrimSpace(cfg.BaseDomain))
	return cfg, nil
}

// tlsConfig loads the control-channel certificate and restricts the handshake
// to TLS 1.3 where possible.
func (c *Config) tlsConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(c.TLSCert, c.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("loading TLS key pair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		// Prefer TLS 1.3; fall back to 1.2 for older clients.
	}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
