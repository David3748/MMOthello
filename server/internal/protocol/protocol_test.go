package protocol

import (
	"reflect"
	"testing"
)

func makeToken(seed byte) [TokenSize]byte {
	var t [TokenSize]byte
	for i := range t {
		t[i] = seed + byte(i)
	}
	return t
}

func makePacked(seed byte) [ChunkPackedBytes]byte {
	var p [ChunkPackedBytes]byte
	for i := range p {
		p[i] = seed + byte(i%17)
	}
	return p
}

func TestRoundTripAllOpcodes(t *testing.T) {
	tests := []Frame{
		Hello{Token: makeToken(1)},
		Subscribe{ChunkID: 17},
		Unsubscribe{ChunkID: 21},
		Place{X: 123, Y: 456},
		Ping{Nonce: 99},
		Welcome{
			SessionID:    42,
			Token:        makeToken(2),
			Team:         1,
			ServerTimeMs: 1700000000123,
		},
		Snapshot{
			ChunkID: 77,
			Version: 9,
			Packed:  makePacked(3),
		},
		DeltaFrame{
			Entries: []Delta{
				{X: 1, Y: 2, Cell: 1},
				{X: 3, Y: 4, Cell: 2},
			},
		},
		PlaceAck{OK: 1, NextAllowedMs: 1700000000999, ErrCode: 0},
		Score{Black: 10, White: 11, Empty: 12},
		Pong{Nonce: 55, ServerTimeMs: 1700000000111},
		ErrorFrame{Code: 5, Msg: "not authenticated"},
	}

	for _, tt := range tests {
		wire, err := EncodeFrame(tt)
		if err != nil {
			t.Fatalf("encode %T: %v", tt, err)
		}
		got, err := DecodeFrame(wire)
		if err != nil {
			t.Fatalf("decode %T: %v", tt, err)
		}
		if !reflect.DeepEqual(tt, got) {
			t.Fatalf("round-trip mismatch for %T: want %#v got %#v", tt, tt, got)
		}
	}
}
