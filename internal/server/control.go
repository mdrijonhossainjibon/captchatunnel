package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/captchamaster/captchatunnel/internal/auth"
	"github.com/captchamaster/captchatunnel/internal/protocol"
	"github.com/captchamaster/captchatunnel/internal/tunnel"
	"github.com/captchamaster/captchatunnel/internal/version"
)

const (
	handshakeTimeout = 15 * time.Second
	keepAlivePeriod  = 10 * time.Second
	writeTimeout     = 30 * time.Second
)

// handleControlConn is the entry point for every inbound client connection.
func (s *Server) handleControlConn(raw net.Conn) {
	tlsConn, ok := raw.(*tls.Conn)
	if !ok {
		raw.Close()
		return
	}

	// Finish the TLS handshake under a deadline so a slow client cannot pin
	// an fd forever.
	_ = raw.SetDeadline(time.Now().Add(handshakeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		raw.Close()
		return
	}
	_ = raw.SetDeadline(time.Time{})

	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = keepAlivePeriod
	cfg.ConnectionWriteTimeout = writeTimeout
	cfg.LogOutput = io.Discard

	session, err := yamux.Server(tlsConn, cfg)
	if err != nil {
		raw.Close()
		return
	}
	defer session.Close()

	s.serveSession(session)
}

// serveSession performs the control handshake on the first accepted stream and
// then runs the heartbeat loop for the lifetime of the tunnel.
func (s *Server) serveSession(session *yamux.Session) {
	stream, err := session.Accept()
	if err != nil {
		return
	}
	defer stream.Close()

	// Bound the handshake; clear the deadline once the tunnel is established.
	_ = stream.SetDeadline(time.Now().Add(handshakeTimeout))
	br := bufio.NewReader(stream)
	bw := bufio.NewWriter(stream)

	t, err := s.handshake(session, br, bw)
	if err != nil {
		// A rejection (or protocol error) is intentionally silent here; the
		// client receives the structured error over the wire.
		_ = bw.Flush()
		return
	}

	_ = stream.SetDeadline(time.Time{})
	defer s.cleanupTunnel(t, "client disconnected")

	if t.Type == tunnel.TypeTCP {
		if err := s.startTCPListener(t); err != nil {
			_ = s.sendRegistered(bw, false, nil, "cannot start TCP listener: "+err.Error())
			_ = bw.Flush()
			return
		}
	}

	if err := s.sendRegistered(bw, true, t, ""); err != nil {
		return
	}
	if err := bw.Flush(); err != nil {
		return
	}

	s.controlLoop(t, br, bw)
}

// handshake authenticates the client and registers its tunnel.
func (s *Server) handshake(session *yamux.Session, br *bufio.Reader, bw *bufio.Writer) (*Tunnel, error) {
	// 1. hello
	msg, err := protocol.ReadMessage(br)
	if err != nil {
		return nil, err
	}
	if msg.Type != protocol.TypeHello {
		return nil, fmt.Errorf("expected hello, got %q", msg.Type)
	}
	var hello protocol.Hello
	if err := msg.Decode(&hello); err != nil {
		return nil, err
	}
	if hello.Version != version.ProtocolVersion {
		_ = s.sendRegistered(bw, false, nil, "unsupported protocol version")
		return nil, fmt.Errorf("unsupported protocol version %d", hello.Version)
	}

	// 2. challenge
	serverNonce := auth.RandomHex(16)
	challengeMsg, _ := protocol.New(protocol.TypeChallenge, protocol.Challenge{ServerNonce: serverNonce})
	if err := protocol.WriteMessage(bw, challengeMsg); err != nil {
		return nil, err
	}
	if err := bw.Flush(); err != nil {
		return nil, err
	}

	// 3. register
	msg, err = protocol.ReadMessage(br)
	if err != nil {
		return nil, err
	}
	if msg.Type != protocol.TypeRegister {
		return nil, fmt.Errorf("expected register, got %q", msg.Type)
	}
	var reg protocol.Register
	if err := msg.Decode(&reg); err != nil {
		return nil, err
	}

	if !auth.VerifyProof(s.cfg.Token, serverNonce, hello.ClientNonce, reg.Auth) {
		_ = s.sendRegistered(bw, false, nil, "unauthorized: invalid tunnel token")
		return nil, fmt.Errorf("unauthorized token proof")
	}

	typ, ok := tunnel.ValidType(reg.Proto)
	if !ok {
		_ = s.sendRegistered(bw, false, nil, "invalid proto (want http or tcp)")
		return nil, fmt.Errorf("invalid proto %q", reg.Proto)
	}

	t := &Tunnel{
		ID:          auth.RandomHex(8),
		Type:        typ,
		Target:      reg.Target,
		Region:      reg.Region,
		TokenHash:   auth.HashToken(s.cfg.Token),
		Session:     session,
		ConnectedAt: time.Now(),
	}
	t.Touch()

	evicted, err := s.reg.Register(t, reg.Subdomain, s.cfg.TCPPortMin, s.cfg.TCPPortMax)
	if evicted != nil {
		// A reconnect reclaimed this token's tunnel: retire the stale one
		// whether or not the new registration succeeds.
		s.closeTCPListener(evicted)
		if evicted.Session != nil {
			_ = evicted.Session.GoAway()
		}
	}
	if err != nil {
		_ = s.sendRegistered(bw, false, nil, err.Error())
		return nil, err
	}

	return t, nil
}

// sendRegistered writes a Registered response for the given outcome.
func (s *Server) sendRegistered(bw *bufio.Writer, ok bool, t *Tunnel, errMsg string) error {
	resp := protocol.Registered{OK: ok, Error: errMsg}
	if ok && t != nil {
		resp.TunnelID = t.ID
		resp.Subdomain = t.Subdomain
		resp.PublicURL = s.publicURL(t)
		resp.AssignedPort = t.Port
	}
	resp.HeartbeatSec = int(s.cfg.HeartbeatInterval / time.Second)
	msg, err := protocol.New(protocol.TypeRegistered, resp)
	if err != nil {
		return err
	}
	return protocol.WriteMessage(bw, msg)
}

// controlLoop services heartbeats until the client goes away.
func (s *Server) controlLoop(t *Tunnel, br *bufio.Reader, bw *bufio.Writer) {
	for {
		msg, err := protocol.ReadMessage(br)
		if err != nil {
			return
		}
		switch msg.Type {
		case protocol.TypePing:
			t.Touch()
			pong, _ := protocol.New(protocol.TypePong, struct{}{})
			if err := protocol.WriteMessage(bw, pong); err != nil {
				return
			}
			if err := bw.Flush(); err != nil {
				return
			}
		case protocol.TypeClose:
			return
		default:
			// Unknown control messages are ignored for forward compatibility.
		}
	}
}

// publicURL builds the public address for a tunnel.
func (s *Server) publicURL(t *Tunnel) string {
	if t.Type == tunnel.TypeHTTP {
		return fmt.Sprintf("https://%s.%s", t.Subdomain, s.cfg.BaseDomain)
	}
	return fmt.Sprintf("tcp://%s:%d", s.cfg.PublicAddr, t.Port)
}

// subdomainFor extracts the wildcard subdomain from a Host header value.
func (s *Server) subdomainFor(hostname string) string {
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = h
	}
	hostname = toLower(hostname)
	if hostname == s.cfg.BaseDomain {
		return ""
	}
	suffix := "." + s.cfg.BaseDomain
	if len(hostname) > len(suffix) && hostname[len(hostname)-len(suffix):] == suffix {
		sub := hostname[:len(hostname)-len(suffix)]
		if tunnel.ValidSubdomain(sub) {
			return sub
		}
	}
	return ""
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
