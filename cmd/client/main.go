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

	// Defaults can be baked into ~/.captchatunnel/config.json:
	// { "server": "...", "tls_ca": "/path/ca.crt", "tls_server_name": "...", "token": "..." }
	// Flags and environment variables always win over the config file.
	def := loadConfigFile()

	// Accept `captchatunnel 3000` (single port => http tunnel) as a shorthand.
	proto := "http"
	rawTarget := ""
	if len(positionals) >= 2 {
		proto = strings.ToLower(positionals[0])
		rawTarget = positionals[1]
	} else if len(positionals) == 1 {
		rawTarget = positionals[0]
	} else {
		usage(os.Stderr)
		return 2
	}

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

	token := resolveToken(opts.token, def.Token)
	if token == "" {
		fmt.Fprintln(os.Stderr, "no tunnel token provided - using an auto-generated token for this session (works with a no-auth server)")
	}

	server := firstNonEmpty(opts.server, os.Getenv("CAPTCHATUNNEL_SERVER"), def.Server, defaultServer)
	tlsCA := firstNonEmpty(opts.tlsCA, def.TLSCA)
	tlsServerName := firstNonEmpty(opts.tlsServerName, def.TLSServerName)

	cfg := &client.Config{
		Proto:             proto,
		Target:            target,
		Display:           displayTarget(rawTarget),
		Subdomain:         opts.subdomain,
		Token:             token,
		Region:            opts.region,
		ServerAddr:        server,
		TLSCA:             tlsCA,
		TLSSkipVerify:     opts.tlsSkipVerify,
		TLSServerName:     tlsServerName,
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

// fileConfig holds overridable defaults read from ~/.captchatunnel/config.json.
type fileConfig struct {
	Token         string `json:"token"`
	Server        string `json:"server"`
	TLSCA         string `json:"tls_ca"`
	TLSServerName string `json:"tls_server_name"`
}

// loadConfigFile reads optional defaults from ~/.captchatunnel/config.json.
// A missing or invalid file simply yields an empty config.
func loadConfigFile() fileConfig {
	var cfg fileConfig
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	path := filepath.Join(home, ".captchatunnel", "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

// resolveToken resolves the tunnel token from --token, the environment, or a
// config file at ~/.captchatunnel/config.json (in that order). An empty
// result is fine: the client then generates a per-run token and works with a
// no-auth server.
func resolveToken(flagToken, fileToken string) string {
	if flagToken != "" {
		return flagToken
	}
	if env := os.Getenv("CAPTCHATUNNEL_TOKEN"); env != "" {
		return env
	}
	return fileToken
}

func usage(w *os.File) {
	fmt.Fprintf(w, `captchatunnel %s - expose local services through your VPS

Usage:
  captchatunnel http <PORT|HOST:PORT> [options]
  captchatunnel tcp  <PORT|HOST:PORT> [options]
  captchatunnel <PORT|HOST:PORT> [options]   (http is the default)

Examples:
  captchatunnel 3000
  captchatunnel http 3000
  captchatunnel http 5173 --subdomain=myapp
  captchatunnel tcp 22

Options:
  --subdomain=NAME      Request a specific subdomain (default: random)
  --token=SECRET        Tunnel token (or set CAPTCHATUNNEL_TOKEN)
  --region=NAME         Region label (informational)
  --server=ADDR         Server control address (default: %s)
  --tls-ca=FILE         Pin the server CA certificate
  --tls-skip-verify     Skip TLS verification (insecure, testing only)
  --tls-server-name=NAME Override TLS SNI name

Defaults can be set once in ~/.captchatunnel/config.json:
  { "server": "148.113.59.83:4443", "tls_ca": "C:/.../ca-coolify.crt",
    "tls_server_name": "redy.captchamaster.org", "token": "optional" }
Then simply run "captchatunnel 3000" and it uses them automatically.
  --version             Print version and exit
  --help                Show this help
`, version.Version, defaultServer)
}
