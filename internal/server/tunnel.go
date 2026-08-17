package server

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/captchamaster/captchatunnel/internal/tunnel"
)

// Tunnel is a single active tunnel registered by a client. Each tunnel is
// backed by one multiplexed session; individual public connections become
// additional yamux streams on that session.
type Tunnel struct {
	ID        string
	Type      tunnel.Type
	Subdomain string
	Target    string
	Region    string
	TokenHash string

	// Port is the assigned public TCP port (TCP tunnels only).
	Port int

	// listener is the dedicated public TCP listener for this tunnel (TCP
	// tunnels only). Guarded by Server.mu.
	listener net.Listener

	Session *yamux.Session

	ConnectedAt time.Time
	lastSeen    atomic.Int64
}

// Touch marks the tunnel as alive right now.
func (t *Tunnel) Touch() {
	t.lastSeen.Store(time.Now().UnixNano())
}

// LastSeen returns the last time Touch was called.
func (t *Tunnel) LastSeen() time.Time {
	return time.Unix(0, t.lastSeen.Load())
}
