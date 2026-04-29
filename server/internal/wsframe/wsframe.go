// Package wsframe implements a minimal RFC 6455 WebSocket frame reader and
// writer. Only the binary, text, ping, pong, and close opcodes are handled;
// payloads up to 64 KiB are supported (sufficient for our 625-byte snapshots
// and small command frames). Server is the receiver, so all client frames
// must be masked per RFC 6455 §5.3.
package wsframe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

const MaxPayload = 64 * 1024

var (
	ErrFrameTooLarge   = errors.New("ws frame too large")
	ErrUnmaskedClient  = errors.New("client frame not masked")
	ErrUnsupportedRsv  = errors.New("unsupported reserved bits")
	ErrFragmented      = errors.New("fragmented frames not supported")
	ErrUnexpectedClose = errors.New("unexpected close")
)

// Frame is a single decoded application or control frame.
type Frame struct {
	Opcode  byte
	Payload []byte
}

// Read reads one frame from r. Client-to-server frames must be masked;
// server-to-client frames must not be (we use Write for that direction).
func Read(r io.Reader) (Frame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	fin := hdr[0]&0x80 != 0
	rsv := hdr[0] & 0x70
	op := hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	plen := int(hdr[1] & 0x7F)

	if rsv != 0 {
		return Frame{}, ErrUnsupportedRsv
	}
	if !fin {
		return Frame{}, ErrFragmented
	}
	if !masked {
		return Frame{}, ErrUnmaskedClient
	}

	switch plen {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return Frame{}, err
		}
		plen = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return Frame{}, err
		}
		v := binary.BigEndian.Uint64(ext[:])
		if v > uint64(MaxPayload) {
			return Frame{}, ErrFrameTooLarge
		}
		plen = int(v)
	}
	if plen > MaxPayload {
		return Frame{}, ErrFrameTooLarge
	}

	var maskKey [4]byte
	if _, err := io.ReadFull(r, maskKey[:]); err != nil {
		return Frame{}, err
	}

	payload := make([]byte, plen)
	if plen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
		for i := 0; i < plen; i++ {
			payload[i] ^= maskKey[i&3]
		}
	}
	return Frame{Opcode: op, Payload: payload}, nil
}

// Write writes a single unmasked server-to-client frame.
func Write(w io.Writer, op byte, payload []byte) error {
	plen := len(payload)
	if plen > MaxPayload {
		return ErrFrameTooLarge
	}
	hdrLen := 2
	switch {
	case plen >= 65536:
		return ErrFrameTooLarge
	case plen >= 126:
		hdrLen = 4
	}
	hdr := make([]byte, hdrLen)
	hdr[0] = 0x80 | (op & 0x0F)
	if plen < 126 {
		hdr[1] = byte(plen)
	} else {
		hdr[1] = 126
		binary.BigEndian.PutUint16(hdr[2:], uint16(plen))
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if plen == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// WriteCloseStatus writes a Close frame with the given status code.
func WriteCloseStatus(w io.Writer, code uint16) error {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], code)
	return Write(w, OpClose, b[:])
}

// String for debug only.
func (f Frame) String() string {
	return fmt.Sprintf("Frame{op=0x%X len=%d}", f.Opcode, len(f.Payload))
}
