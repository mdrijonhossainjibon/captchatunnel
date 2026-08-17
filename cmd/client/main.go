package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/captchamaster/captchatunnel/internal/client"
	"github.com/captchamaster/captchatunnel/internal/version"
)

const (
	defaultServer = "redy.captchamaster.org:4443"

	heartbeatInterval = 5 * time.Second
	heartbeatTimeout  = 15 * time.Second
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, positionals, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n\n", err)
		usage(os.Stderr)
		return 2
	}

	if opts.help {
		usage(os.Stdout)
		return 0
	}
	if opts.showVersion {
		fmt.Println(version.Version)
		return 0
	}

	if len(positionals) != 2 {
		usage(os.Stderr)
		return 2
	}
	proto := strings.ToLower(positionals[0])
	rawTarget := positionals[1]

	if proto != "http" && proto != "tcp" {
		fmt.Fprintf(os.Stderr, "error: unsupported protocol %q (use http or tcp)\n\n", proto)
		usage(os.Stderr)
		return 2
	}

	target, err := client.ResolveTarget(rawTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	token := resolveToken(opts.token)
	if token == "" {
		fmt.Fprintln(os.Stderr, "no tunnel token provided - using an auto-generated token for this session (works with a no-auth server)")
	}

	server := opts.server
	if server == "" {
		server = firstNonEmpty(os.Getenv("CAPTCHATUNNEL_SERVER"), defaultServer)
	}

	cfg := &client.Config{
		Proto:             proto,
		Target:            target,
		Display:           displayTarget(rawTarget),
		Subdomain:         opts.subdomain,
		Token:             token,
		Region:            opts.region,
		ServerAddr:        server,
		TLSCA:             opts.tlsCA,
		TLSSkipVerify:     opts.tlsSkipVerify,
		TLSServerName:     opts.tlsServerName,
		HeartbeatInterval: heartbeatInterval,
		HeartbeatTimeout:  heartbeatTimeout,
	}

	c, err := client.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := c.Run(ctx); err != nil {
		return 1
	}
	return 0
}

// displayTarget renders a friendly forwarding address for the status UI.
func displayTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if isDigits(raw) {
		return "localhost:" + raw
	}
	return raw
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveToken resolves the tunnel token from --token, the environment, or a
// config file at ~/.captchatunnel/config.json (in that order). An empty
// result is fine: the client then generates a per-run token and works with a
// no-auth server.
func resolveToken(flagToken string) string {
	if flagToken != "" {
		return flagToken
	}
	if env := os.Getenv("CAPTCHATUNNEL_TOKEN"); env != "" {
		return env
	}
	return tokenFromConfigFile()
}

func tokenFromConfigFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".captchatunnel", "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		Token  string `json:"token"`
		Server string `json:"server"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return ""
	}
	return cfg.Token
}

func usage(w *os.File) {
	fmt.Fprintf(w, `captchatunnel %s - expose local services through your VPS

Usage:
  captchatunnel http <PORT|HOST:PORT> [options]
  captchatunnel tcp  <PORT|HOST:PORT> [options]

Examples:
  captchatunnel http 3000
  captchatunnel http 5173 --subdomain=myapp
  captchatunnel tcp 22
  captchatunnel tcp 25565 --region=sg

Options:
  --subdomain=NAME      Request a specific subdomain (default: random)
  --token=SECRET        Tunnel token (or set CAPTCHATUNNEL_TOKEN)
  --region=NAME         Region label (informational)
  --server=ADDR         Server control address (default: %s)
  --tls-ca=FILE         Pin the server CA certificate
  --tls-skip-verify     Skip TLS verification (insecure, testing only)
  --tls-server-name=NAME Override TLS SNI name
  --version             Print version and exit
  --help                Show this help
`, version.Version, defaultServer)
}
