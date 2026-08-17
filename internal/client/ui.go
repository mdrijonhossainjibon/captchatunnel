package client

import (
	"fmt"
	"io"
	"os"

	"github.com/captchamaster/captchatunnel/internal/protocol"
)

// stderr returns the writer used for status/log messages so that programmatic
// consumers of stdout see only the public URL block.
func stderr() io.Writer { return os.Stderr }

// printConnected renders the Cloudflare-Tunnel-style status block.
func printConnected(cfg *Config, resp *protocol.Registered) {
	fmt.Println()
	fmt.Println("\u2713 Connected")
	fmt.Println()
	fmt.Println("Public URL:")
	fmt.Printf("  %s\n", resp.PublicURL)
	fmt.Println()
	fmt.Println("Forwarding:")
	fmt.Printf("  %s://%s\n", cfg.Proto, cfg.Display)
	fmt.Println()
	fmt.Println("Status:")
	fmt.Println("  Online")
	fmt.Println()
}
