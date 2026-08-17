package protocol

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
)

// maxFrameSize bounds control message size. Data streams are unbounded.
const maxFrameSize = 1 << 20 // 1 MiB

// WriteMessage writes a length-prefixed JSON message to w.
func WriteMessage(w io.Writer, m *Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(b) > maxFrameSize {
		return io.ErrShortWrite
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// ReadMessage reads a single length-prefixed JSON message from r.
func ReadMessage(r *bufio.Reader) (*Message, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > maxFrameSize {
		return nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	var m Message
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
