// Package tunnel holds shared tunnel type definitions and helpers used by both
// the server and the client.
package tunnel

import (
	"crypto/rand"
	"regexp"
	"strings"
)

// Type is the tunnel protocol.
type Type string

const (
	TypeHTTP Type = "http"
	TypeTCP  Type = "tcp"
)

// ValidType normalizes and validates a tunnel protocol string.
func ValidType(s string) (Type, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "http":
		return TypeHTTP, true
	case "tcp":
		return TypeTCP, true
	default:
		return "", false
	}
}

var subdomainRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidSubdomain reports whether s is a legal wildcard subdomain label
// (RFC 1035-ish: lowercase letters, digits and internal hyphens, <= 63 chars).
func ValidSubdomain(s string) bool {
	return len(s) > 0 && len(s) <= 63 && subdomainRE.MatchString(s)
}

const subdomainChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// RandomSubdomain returns a fresh 6-character lowercase subdomain using
// crypto/rand (unpredictable, avoids collisions with high probability).
func RandomSubdomain() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic("tunnel: crypto/rand failed: " + err.Error())
	}
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = subdomainChars[int(c)%len(subdomainChars)]
	}
	return string(out)
}
