package nativemsg

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

type msg struct {
	Kind string `json:"kind"`
	N    int    `json:"n"`
}

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := msg{Kind: "resolve", N: 7}
	if err := Write(&buf, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out msg
	if err := Read(&buf, &out); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != in {
		t.Errorf("got %+v, want %+v", out, in)
	}
}

func TestReadHonorsLengthPrefix(t *testing.T) {
	// Two messages back to back must decode independently.
	var buf bytes.Buffer
	Write(&buf, msg{Kind: "a", N: 1})
	Write(&buf, msg{Kind: "b", N: 2})

	var a, b msg
	if err := Read(&buf, &a); err != nil || a.Kind != "a" {
		t.Fatalf("first: %+v err=%v", a, err)
	}
	if err := Read(&buf, &b); err != nil || b.Kind != "b" {
		t.Fatalf("second: %+v err=%v", b, err)
	}
}

func TestReadEOFAtBoundary(t *testing.T) {
	var buf bytes.Buffer
	var out msg
	if err := Read(&buf, &out); err != io.EOF {
		t.Errorf("expected EOF on empty stream, got %v", err)
	}
}

func TestReadRejectsOversize(t *testing.T) {
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], MaxMessageSize+1)
	var out msg
	if err := Read(bytes.NewReader(hdr[:]), &out); err == nil {
		t.Error("expected error for oversize message")
	}
}
