package server

import (
	"net"
	"strconv"

	"github.com/captchamaster/captchatunnel/internal/relay"
)

// startTCPListener binds a dedicated public port for a TCP tunnel and starts
// accepting connections on it.
func (s *Server) startTCPListener(t *Tunnel) error {
	addr := net.JoinHostPort(s.cfg.TCPAddr, strconv.Itoa(t.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	t.listener = ln
	s.mu.Unlock()

	go s.acceptTCP(ln, t)
	return nil
}

func (s *Server) acceptTCP(ln net.Listener, t *Tunnel) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.relayTCP(conn, t)
	}
}

// relayTCP forwards one inbound public TCP connection to the tunnel's local
// target over a fresh multiplexed stream.
func (s *Server) relayTCP(conn net.Conn, t *Tunnel) {
	defer conn.Close()

	if t.Session == nil || t.Session.IsClosed() {
		return
	}
	stream, err := t.Session.Open()
	if err != nil {
		return
	}
	defer stream.Close()

	relay.Relay(conn, stream)
}
