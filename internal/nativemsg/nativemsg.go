// Package nativemsg implements Chrome's native-messaging wire format: each
// message is a 4-byte little-endian uint32 length prefix followed by that many
// bytes of UTF-8 JSON. Chrome writes requests to the host's stdin and reads
// responses from stdout.
package nativemsg

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MaxMessageSize bounds an incoming message (Chrome's own cap is 1 MB for
// messages sent *to* the extension; we allow a bit more for inbound page bodies).
const MaxMessageSize = 8 << 20 // 8 MiB

// Read reads one length-prefixed message from r and unmarshals it into v.
// Returns io.EOF cleanly when the stream closes between messages.
func Read(r io.Reader, v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err // io.EOF at a clean boundary
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n == 0 {
		return fmt.Errorf("zero-length message")
	}
	if n > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("short read: %w", err)
	}
	return json.Unmarshal(buf, v)
}

// Write marshals v and writes it to w with the 4-byte length prefix.
func Write(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes", len(data))
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
