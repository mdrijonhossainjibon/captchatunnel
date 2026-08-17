// Package auth implements token possession proofs and replay protection for the
// tunnel handshake.
//
// The token itself is never transmitted. After the TLS connection is
// established the server issues a fresh random nonce; the client proves it
// holds the token by returning HMAC-SHA256(token, serverNonce ":" clientNonce).
// Because the nonce is fresh for every handshake, a captured proof cannot be
// replayed.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// RandomHex returns n random bytes encoded as a lowercase hex string.
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// ComputeProof builds the authentication proof from the token and both nonces.
func ComputeProof(token, serverNonce, clientNonce string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(serverNonce))
	mac.Write([]byte(":"))
	mac.Write([]byte(clientNonce))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyProof reports whether proof matches the expected value, using a
// constant-time comparison to avoid timing side channels.
func VerifyProof(token, serverNonce, clientNonce, proof string) bool {
	expected := ComputeProof(token, serverNonce, clientNonce)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(proof)) == 1
}

// HashToken returns a stable, non-reversible identifier for a token. It is
// used to bind a tunnel to its owner so that reconnects reclaim the same
// subdomain. Only the hash is stored server-side, never the token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ValidToken reports whether a token is strong enough to use.
func ValidToken(token string) bool {
	return len(strings.TrimSpace(token)) >= 16
}
