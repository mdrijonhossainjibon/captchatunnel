package server

import (
	"errors"
	"fmt"
	"sync"

	"github.com/captchamaster/captchatunnel/internal/tunnel"
)

var (
	ErrSubdomainTaken = errors.New("subdomain already in use")
	ErrNoFreePort     = errors.New("no free TCP port available")
)

// Registry tracks every live tunnel and resolves public addresses to tunnels.
type Registry struct {
	mu      sync.RWMutex
	bySub   map[string]*Tunnel // HTTP tunnels keyed by subdomain
	byPort  map[int]*Tunnel    // TCP tunnels keyed by public port
	byToken map[string]*Tunnel // keyed by token hash (for reconnect reclaim)
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		bySub:   make(map[string]*Tunnel),
		byPort:  make(map[int]*Tunnel),
		byToken: make(map[string]*Tunnel),
	}
}

// Register inserts t, allocating a subdomain and (for TCP) a public port.
//
// If t.TokenHash is already owned by another live tunnel, that tunnel is
// evicted (removed and returned) so a client reconnect can reclaim its
// subdomain/port. One token == one active tunnel.
func (r *Registry) Register(t *Tunnel, requestedSubdomain string, minPort, maxPort int) (*Tunnel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var evicted *Tunnel
	if t.TokenHash != "" {
		if old, exists := r.byToken[t.TokenHash]; exists && old != t {
			evicted = old
			r.removeLocked(old)
		}
	}

	if t.Type == tunnel.TypeHTTP {
		sub := requestedSubdomain
		if sub == "" {
			sub = tunnel.RandomSubdomain()
		}
		if !tunnel.ValidSubdomain(sub) {
			return evicted, fmt.Errorf("invalid subdomain %q", sub)
		}
		if _, exists := r.bySub[sub]; exists {
			return evicted, ErrSubdomainTaken
		}
		t.Subdomain = sub
		r.bySub[sub] = t
	} else {
		port, err := r.allocPortLocked(minPort, maxPort)
		if err != nil {
			return evicted, err
		}
		t.Port = port
		r.byPort[port] = t
	}

	if t.TokenHash != "" {
		r.byToken[t.TokenHash] = t
	}
	return evicted, nil
}

// Unregister removes t and releases its public resources.
func (r *Registry) Unregister(t *Tunnel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeLocked(t)
}

func (r *Registry) removeLocked(t *Tunnel) {
	if t.Type == tunnel.TypeHTTP {
		if cur, ok := r.bySub[t.Subdomain]; ok && cur == t {
			delete(r.bySub, t.Subdomain)
		}
	} else {
		if cur, ok := r.byPort[t.Port]; ok && cur == t {
			delete(r.byPort, t.Port)
		}
	}
	if t.TokenHash != "" {
		if cur, ok := r.byToken[t.TokenHash]; ok && cur == t {
			delete(r.byToken, t.TokenHash)
		}
	}
}

// LookupSub returns the HTTP tunnel for a subdomain, if any.
func (r *Registry) LookupSub(sub string) *Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bySub[sub]
}

// LookupPort returns the TCP tunnel for a public port, if any.
func (r *Registry) LookupPort(port int) *Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byPort[port]
}

// LookupToken returns the tunnel owned by a token hash, if any.
func (r *Registry) LookupToken(hash string) *Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byToken[hash]
}

// All returns a snapshot of all active tunnels.
func (r *Registry) All() []*Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tunnel, 0, len(r.bySub)+len(r.byPort))
	for _, t := range r.bySub {
		out = append(out, t)
	}
	for _, t := range r.byPort {
		out = append(out, t)
	}
	return out
}

// Count returns the number of active tunnels.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.bySub) + len(r.byPort)
}

func (r *Registry) allocPortLocked(minPort, maxPort int) (int, error) {
	for p := minPort; p <= maxPort; p++ {
		if _, taken := r.byPort[p]; !taken {
			return p, nil
		}
	}
	return 0, ErrNoFreePort
}
