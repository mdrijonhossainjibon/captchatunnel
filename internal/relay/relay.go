// Package relay provides a bidirectional byte pipe between two network
// connections. It is used to bridge a public connection and a multiplexed
// stream in both the server and the client.
package relay

import (
	"io"
	"net"
	"sync"
)

// Relay copies bytes in both directions between a and b until one direction
// reaches EOF, then closes the pair. It is safe for arbitrary full-duplex
// traffic (HTTP, WebSocket, TLS-in-TCP, SSH, raw TCP).
func Relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		CopyAndClose(a, b)
	}()
	go func() {
		defer wg.Done()
		CopyAndClose(b, a)
	}()
	wg.Wait()
}

// CopyAndClose drains src into dst and then closes dst's write side when the
// underlying transport supports half-close, otherwise closes it fully.
func CopyAndClose(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if hc, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = hc.CloseWrite()
		return
	}
	_ = dst.Close()
}
