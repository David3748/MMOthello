package wsframe

import (
	"bytes"
	"testing"
)

func TestRoundTripBinary(t *testing.T) {
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	var buf bytes.Buffer
	if err := Write(&buf, OpBinary, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Convert server-style unmasked frame into a masked client frame for the
	// reader, since Read enforces masked input.
	masked := mask(buf.Bytes())
	got, err := Read(bytes.NewReader(masked))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Opcode != OpBinary || !bytes.Equal(got.Payload, want) {
		t.Fatalf("payload mismatch: got=%+v", got)
	}
}

func TestRejectsUnmaskedClientFrame(t *testing.T) {
	var buf bytes.Buffer
	_ = Write(&buf, OpBinary, []byte{1, 2, 3})
	if _, err := Read(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected unmasked rejection")
	}
}

func TestExtendedLength(t *testing.T) {
	want := bytes.Repeat([]byte{0xAB}, 200)
	var buf bytes.Buffer
	if err := Write(&buf, OpBinary, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	masked := mask(buf.Bytes())
	got, err := Read(bytes.NewReader(masked))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got.Payload, want) {
		t.Fatalf("payload mismatch (len got=%d want=%d)", len(got.Payload), len(want))
	}
}

// mask rewrites a server-originated frame into a client-originated masked
// frame so we can feed it back through Read for testing.
func mask(serverFrame []byte) []byte {
	if len(serverFrame) < 2 {
		panic("frame too short")
	}
	hdr := append([]byte(nil), serverFrame[:2]...)
	plen := int(hdr[1] & 0x7F)
	cursor := 2
	switch plen {
	case 126:
		hdr = append(hdr, serverFrame[2:4]...)
		plen = int(uint16(serverFrame[2])<<8 | uint16(serverFrame[3]))
		cursor = 4
	case 127:
		hdr = append(hdr, serverFrame[2:10]...)
		var v uint64
		for i := 0; i < 8; i++ {
			v = (v << 8) | uint64(serverFrame[2+i])
		}
		plen = int(v)
		cursor = 10
	}
	hdr[1] |= 0x80 // set masked bit
	maskKey := []byte{0x12, 0x34, 0x56, 0x78}
	payload := append([]byte(nil), serverFrame[cursor:]...)
	if len(payload) != plen {
		panic("plen mismatch")
	}
	for i := 0; i < plen; i++ {
		payload[i] ^= maskKey[i&3]
	}
	out := append(hdr, maskKey...)
	out = append(out, payload...)
	return out
}
