// Package protocol defines the wire messages exchanged between the tunnel
// client and the tunnel server over the control stream.
//
// Framing: every control message is a 4-byte big-endian length prefix followed
// by a JSON payload. Data streams (the actual forwarded traffic) are NOT
// framed here; they are raw, bidirectional byte pipes multiplexed with yamux.
package protocol

import "encoding/json"

// Control message types.
const (
	TypeHello      = "hello"
	TypeChallenge  = "challenge"
	TypeRegister   = "register"
	TypeRegistered = "registered"
	TypePing       = "ping"
	TypePong       = "pong"
	TypeClose      = "close"
)

// Message is the envelope for every control message.
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Hello is the first client message. It advertises the protocol version and a
// fresh client nonce used later for the authentication proof.
type Hello struct {
	Version     int    `json:"version"`
	ClientNonce string `json:"client_nonce"`
	Region      string `json:"region,omitempty"`
}

// Challenge is the server's response carrying a fresh server nonce. The client
// must prove possession of the token by combining both nonces.
type Challenge struct {
	ServerNonce string `json:"server_nonce"`
}

// Register is the client's request to open a tunnel.
type Register struct {
	Proto     string `json:"proto"`               // "http" | "tcp"
	Subdomain string `json:"subdomain,omitempty"` // empty => server picks one
	Target    string `json:"target"`              // local addr, e.g. 127.0.0.1:3000
	Region    string `json:"region,omitempty"`
	Owner     string `json:"owner,omitempty"` // client token (used for ownership/reclaim)
	Auth      string `json:"auth"`            // HMAC-SHA256(token, serverNonce:clientNonce)
}

// Registered is the server's response. On success it carries the public URL
// the user should use.
type Registered struct {
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	TunnelID      string `json:"tunnel_id,omitempty"`
	Subdomain     string `json:"subdomain,omitempty"`
	PublicURL     string `json:"public_url,omitempty"`
	AssignedPort  int    `json:"assigned_port,omitempty"`
	HeartbeatSec  int    `json:"heartbeat_sec,omitempty"`
}

// Close is sent by either side to gracefully terminate a tunnel.
type Close struct {
	Reason string `json:"reason"`
}

// New returns an envelope for the given type. The caller marshals the concrete
// payload into Data first.
func New(typ string, data any) (*Message, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Message{Type: typ, Data: raw}, nil
}

// Decode unmarshals m.Data into dst.
func (m *Message) Decode(dst any) error {
	return json.Unmarshal(m.Data, dst)
}
