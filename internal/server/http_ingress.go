package server

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/captchamaster/captchatunnel/internal/relay"
)

// maxHeaderSize bounds the HTTP header block we are willing to buffer before
// routing a request.
const maxHeaderSize = 64 * 1024

// handleHTTPConn routes an inbound HTTP(S) connection (proxied here by Nginx)
// to the tunnel whose subdomain matches the Host header. The connection is
// forwarded as a raw byte pipe, so WebSocket, SSE, streaming and chunked
// bodies all pass through unmodified.
func (s *Server) handleHTTPConn(c net.Conn) {
	defer c.Close()

	br := bufio.NewReader(c)

	host, header, err := readHostHeader(br)
	if err != nil {
		return
	}

	sub := s.subdomainFor(host)
	t := s.reg.LookupSub(sub)
	if t == nil || t.Session == nil || t.Session.IsClosed() {
		s.writeNotFound(c)
		return
	}

	stream, err := t.Session.Open()
	if err != nil {
		s.writeNotFound(c)
		return
	}
	defer stream.Close()

	// Flush the buffered header block (and any already-read body bytes) into
	// the stream, then relay the rest of the connection raw.
	if len(header) > 0 {
		if _, err := stream.Write(header); err != nil {
			return
		}
	}
	if br.Buffered() > 0 {
		if _, err := io.CopyN(stream, br, int64(br.Buffered())); err != nil {
			return
		}
	}

	relay.Relay(c, stream)
}

// readHostHeader reads the request head up to (and including) the blank line
// separating headers from the body, returning the Host value and the raw
// header bytes. Any body bytes already buffered remain in br.
func readHostHeader(br *bufio.Reader) (host string, raw []byte, err error) {
	var buf bytes.Buffer
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			buf.Write(line)
		}
		if buf.Len() > maxHeaderSize {
			return "", nil, io.ErrUnexpectedEOF
		}
		if err != nil {
			return "", nil, err
		}
		if host == "" {
			if v, ok := headerField(line, "host"); ok {
				host = v
			}
		}
		// Blank line (end of header block).
		if len(line) == 2 && line[0] == '\r' && line[1] == '\n' {
			return host, buf.Bytes(), nil
		}
		if len(line) == 1 && line[0] == '\n' {
			return host, buf.Bytes(), nil
		}
	}
}

// headerField returns the value of a request header line (case-insensitive
// name match) without the trailing CRLF.
func headerField(line []byte, name string) (string, bool) {
	if len(line) <= len(name) {
		return "", false
	}
	if !strings.EqualFold(string(line[:len(name)]), name) {
		return "", false
	}
	if line[len(name)] != ':' {
		return "", false
	}
	v := strings.TrimSpace(string(line[len(name)+1:]))
	return strings.TrimRight(v, "\r\n"), true
}

func (s *Server) writeNotFound(c net.Conn) {
	body := "tunnel not found\n"
	resp := "HTTP/1.1 404 Not Found\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n" +
		"Connection: close\r\n" +
		"\r\n" +
		body
	_, _ = io.WriteString(c, resp)
}
