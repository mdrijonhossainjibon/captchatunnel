package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"
)

// Server is the tunnel server. It owns the control listener (TLS), the HTTP
// ingress listener, per-port TCP ingress listeners, and the tunnel registry.
type Server struct {
	cfg     *Config
	reg     *Registry
	tlsConf *tls.Config

	mu sync.Mutex // guards Tunnel.listener
}

// New builds a Server from cfg.
func New(cfg *Config) (*Server, error) {
	tlsConf, err := cfg.tlsConfig()
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:     cfg,
		reg:     NewRegistry(),
		tlsConf: tlsConf,
	}, nil
}

// Run starts all listeners and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	controlLn, err := tls.Listen("tcp", s.cfg.ControlAddr, s.tlsConf)
	if err != nil {
		return fmt.Errorf("control listener: %w", err)
	}
	httpLn, err := net.Listen("tcp", s.cfg.HTTPAddr)
	if err != nil {
		controlLn.Close()
		return fmt.Errorf("http ingress listener: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); s.acceptLoop(ctx, controlLn, s.handleControlConn) }()
	wg.Add(1)
	go func() { defer wg.Done(); s.acceptLoop(ctx, httpLn, s.handleHTTPConn) }()
	wg.Add(1)
	go func() { defer wg.Done(); s.cleanupLoop(ctx) }()

	<-ctx.Done()

	controlLn.Close()
	httpLn.Close()
	s.closeAllTCPListeners()
	wg.Wait()
	return nil
}

// acceptLoop accepts connections until ctx is cancelled or the listener
// errors out, dispatching each connection to handler in its own goroutine.
func (s *Server) acceptLoop(ctx context.Context, ln net.Listener, handler func(net.Conn)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				// Listener was closed (shutdown) or hit a fatal error.
				return
			}
		}
		go handler(conn)
	}
}

// cleanupLoop periodically evicts tunnels whose heartbeat has gone silent.
func (s *Server) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.HeartbeatTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			for _, t := range s.reg.All() {
				if now.Sub(t.LastSeen()) > s.cfg.HeartbeatTimeout {
					s.cleanupTunnel(t, "heartbeat timeout")
				}
			}
		}
	}
}

// cleanupTunnel removes t from the registry and releases its TCP listener.
func (s *Server) cleanupTunnel(t *Tunnel, reason string) {
	s.reg.Unregister(t)
	s.closeTCPListener(t)
	if t.Session != nil {
		_ = t.Session.GoAway()
	}
}

// closeAllTCPListeners shuts down every per-port TCP listener on shutdown.
func (s *Server) closeAllTCPListeners() {
	for _, t := range s.reg.All() {
		s.closeTCPListener(t)
	}
}

// closeTCPListener closes only the listener owned by t, so a replaced tunnel
// cannot accidentally close its successor's listener.
func (s *Server) closeTCPListener(t *Tunnel) {
	s.mu.Lock()
	ln := t.listener
	t.listener = nil
	s.mu.Unlock()
	if ln != nil {
		ln.Close()
	}
}
