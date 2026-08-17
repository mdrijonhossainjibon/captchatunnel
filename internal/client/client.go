// Package client implements the tunnel CLI: it connects out to the tunnel
// server, authenticates with the tunnel token, and relays public traffic to a
// local service. It reconnects automatically with exponential backoff.
package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/captchamaster/captchatunnel/internal/auth"
	"github.com/captchamaster/captchatunnel/internal/protocol"
	"github.com/captchamaster/captchatunnel/internal/relay"
	"github.com/captchamaster/captchatunnel/internal/version"
)

const (
	initialBackoff = time.Second
	maxBackoff     = 30 * time.Second
	dialTimeout    = 10 * time.Second
	keepAlivePeriod = 10 * time.Second
	writeTimeout   = 30 * time.Second
)

// RejectError indicates a permanent, non-retryable rejection from the server
// (e.g. bad token or an explicitly requested subdomain that is taken).
type RejectError struct{ Msg string }

func (e *RejectError) Error() string { return e.Msg }

var errDisconnected = errors.New("connection lost")

// Client is a single tunnel session manager.
type Client struct {
	cfg     *Config
	tlsConf *tls.Config

	mu        sync.Mutex
	subdomain string // resolved subdomain, persisted across reconnects
	connected bool   // whether the connected UI has been shown

	lastPong atomic.Int64
}

// New builds a Client from cfg.
func New(cfg *Config) (*Client, error) {
	tlsConf, err := cfg.TLSConfig()
	if err != nil {
		return nil, err
	}
	return &Client{
		cfg:       cfg,
		tlsConf:   tlsConf,
		subdomain: cfg.Subdomain,
	}, nil
}

// Run keeps the tunnel alive until ctx is cancelled, reconnecting with
// exponential backoff after any network failure.
func (c *Client) Run(ctx context.Context) error {
	backoff := initialBackoff
	for {
		err := c.connectOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}

		var rej *RejectError
		if errors.As(err, &rej) {
			// A taken auto-assigned subdomain is recoverable: ask for a new one.
			if c.cfg.Subdomain == "" && isSubdomainTaken(rej.Msg) {
				c.mu.Lock()
				c.subdomain = ""
				c.mu.Unlock()
				fmt.Fprintf(stderr(), "error: %s (retrying with a new subdomain)\n", rej.Msg)
				backoff = initialBackoff
				continue
			}
			fmt.Fprintf(stderr(), "error: %s\n", rej.Msg)
			return rej
		}

		fmt.Fprintf(stderr(), "\nConnection lost (%v). Reconnecting in %s...\n", err, backoff.Round(time.Millisecond*100))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		backoff += time.Duration(rand.Int63n(int64(time.Second))) // jitter
	}
}

// connectOnce establishes one full tunnel session and blocks until it ends.
func (c *Client) connectOnce(ctx context.Context) error {
	d := &net.Dialer{Timeout: dialTimeout}
	conn, err := (&tls.Dialer{NetDialer: d, Config: c.tlsConf}).DialContext(ctx, "tcp", c.cfg.ServerAddr)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = keepAlivePeriod
	cfg.ConnectionWriteTimeout = writeTimeout
	cfg.LogOutput = io.Discard

	session, err := yamux.Client(conn, cfg)
	if err != nil {
		return fmt.Errorf("multiplexer: %w", err)
	}
	defer session.Close()

	stream, err := session.Open()
	if err != nil {
		return fmt.Errorf("open control stream: %w", err)
	}
	defer stream.Close()

	br := bufio.NewReader(stream)
	bw := bufio.NewWriter(stream)

	resp, err := c.handshake(br, bw)
	if err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	if !resp.OK {
		return &RejectError{Msg: resp.Error}
	}

	c.onConnected(resp)

	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }

	go c.acceptLoop(session, closeDone)
	go c.pingLoop(ctx, bw, closeDone)
	go c.readLoop(ctx, br, closeDone)
	go c.livenessLoop(ctx, closeDone)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return errDisconnected
	case <-session.CloseChan():
		return errDisconnected
	}
}

// handshake authenticates with the server and registers the tunnel.
func (c *Client) handshake(br *bufio.Reader, bw *bufio.Writer) (*protocol.Registered, error) {
	clientNonce := auth.RandomHex(16)

	hello, _ := protocol.New(protocol.TypeHello, protocol.Hello{
		Version:     version.ProtocolVersion,
		ClientNonce: clientNonce,
		Region:      c.cfg.Region,
	})
	if err := protocol.WriteMessage(bw, hello); err != nil {
		return nil, err
	}
	if err := bw.Flush(); err != nil {
		return nil, err
	}

	msg, err := protocol.ReadMessage(br)
	if err != nil {
		return nil, err
	}
	if msg.Type != protocol.TypeChallenge {
		return nil, fmt.Errorf("expected challenge, got %q", msg.Type)
	}
	var ch protocol.Challenge
	if err := msg.Decode(&ch); err != nil {
		return nil, err
	}

	c.mu.Lock()
	requested := c.subdomain
	c.mu.Unlock()

	reg, _ := protocol.New(protocol.TypeRegister, protocol.Register{
		Proto:     c.cfg.Proto,
		Subdomain: requested,
		Target:    c.cfg.Target,
		Region:    c.cfg.Region,
		Auth:      auth.ComputeProof(c.cfg.Token, ch.ServerNonce, clientNonce),
	})
	if err := protocol.WriteMessage(bw, reg); err != nil {
		return nil, err
	}
	if err := bw.Flush(); err != nil {
		return nil, err
	}

	msg, err = protocol.ReadMessage(br)
	if err != nil {
		return nil, err
	}
	if msg.Type != protocol.TypeRegistered {
		return nil, fmt.Errorf("expected registered, got %q", msg.Type)
	}
	var resp protocol.Registered
	if err := msg.Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// onConnected records the assigned subdomain and prints the status UI once.
func (c *Client) onConnected(resp *protocol.Registered) {
	c.mu.Lock()
	c.subdomain = resp.Subdomain
	first := !c.connected
	c.connected = true
	c.mu.Unlock()

	if first {
		printConnected(c.cfg, resp)
	} else {
		fmt.Fprintf(stderr(), "Reconnected: %s\n", resp.PublicURL)
	}
}

// acceptLoop handles inbound multiplexed streams opened by the server for each
// public connection.
func (c *Client) acceptLoop(session *yamux.Session, closeDone func()) {
	for {
		stream, err := session.Accept()
		if err != nil {
			closeDone()
			return
		}
		go c.handleStream(stream)
	}
}

func (c *Client) handleStream(stream *yamux.Stream) {
	defer stream.Close()

	conn, err := net.DialTimeout("tcp", c.cfg.Target, dialTimeout)
	if err != nil {
		return
	}
	defer conn.Close()

	relay.Relay(conn, stream)
}

// pingLoop emits heartbeats so the server can detect a dead client.
func (c *Client) pingLoop(ctx context.Context, bw *bufio.Writer, closeDone func()) {
	t := time.NewTicker(c.cfg.HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m, _ := protocol.New(protocol.TypePing, struct{}{})
			if err := protocol.WriteMessage(bw, m); err != nil {
				closeDone()
				return
			}
			if err := bw.Flush(); err != nil {
				closeDone()
				return
			}
		}
	}
}

// readLoop consumes server messages (pongs, close) on the control stream.
func (c *Client) readLoop(ctx context.Context, br *bufio.Reader, closeDone func()) {
	for {
		msg, err := protocol.ReadMessage(br)
		if err != nil {
			closeDone()
			return
		}
		switch msg.Type {
		case protocol.TypePong:
			c.lastPong.Store(time.Now().UnixNano())
		case protocol.TypeClose:
			closeDone()
			return
		}
	}
}

// livenessLoop tears down the session if the server stops answering pongs.
func (c *Client) livenessLoop(ctx context.Context, closeDone func()) {
	c.lastPong.Store(time.Now().UnixNano())
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if time.Since(time.Unix(0, c.lastPong.Load())) > c.cfg.HeartbeatTimeout {
				closeDone()
				return
			}
		}
	}
}

func isSubdomainTaken(msg string) bool {
	return strings.Contains(msg, "subdomain already in use") || strings.Contains(msg, "already in use")
}
