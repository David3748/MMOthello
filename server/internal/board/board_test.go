package board

import "testing"

func TestGetSetRoundTrip(t *testing.T) {
	b := NewBoard()
	values := []Cell{CellEmpty, CellBlack, CellWhite, CellReserved}
	for y := 0; y < BoardSize; y++ {
		for x := 0; x < BoardSize; x++ {
			want := values[(x+y)%len(values)]
			if !b.Set(x, y, want) {
				t.Fatalf("set failed at %d,%d", x, y)
			}
			got := b.Get(x, y)
			if got != want {
				t.Fatalf("round-trip mismatch at %d,%d: got=%d want=%d", x, y, got, want)
			}
		}
	}
}

func TestPackedLayoutMSBFirst(t *testing.T) {
	b := NewBoard()
	_ = b.Set(0, 0, CellBlack)
	_ = b.Set(1, 0, CellWhite)
	_ = b.Set(2, 0, CellEmpty)
	_ = b.Set(3, 0, CellReserved)

	got := b.cells[0]
	// 01 10 00 11 => 0x63
	if got != 0x63 {
		t.Fatalf("unexpected packed byte: got=0x%02x want=0x63", got)
	}
}

func TestChunkMappingAndBounds(t *testing.T) {
	id := ChunkOf(0, 0)
	if id != 0 {
		t.Fatalf("got id %d, want 0", id)
	}
	last := ChunkOf(BoardSize-1, BoardSize-1)
	if last != TotalChunks-1 {
		t.Fatalf("got id %d, want %d", last, TotalChunks-1)
	}

	x0, y0, x1, y1 := ChunkBounds(last)
	if x0 != 950 || y0 != 950 || x1 != 999 || y1 != 999 {
		t.Fatalf("unexpected chunk bounds: %d,%d -> %d,%d", x0, y0, x1, y1)
	}
}

func TestPackChunkMatchesGetCells(t *testing.T) {
	b := NewBoard()
	// Sprinkle a known pattern in chunk 0 (0..49 x 0..49).
	for i := 0; i < 50; i++ {
		_ = b.Set(i, 0, CellBlack)
		_ = b.Set(i, 49, CellWhite)
	}
	_ = b.Set(0, 0, CellWhite)
	_ = b.Set(49, 49, CellBlack)

	out := make([]byte, 625)
	b.PackChunk(0, out)

	// Verify every cell decodes back from the packed buffer.
	for ly := 0; ly < ChunkSize; ly++ {
		for lx := 0; lx < ChunkSize; lx++ {
			i := ly*ChunkSize + lx
			shift := uint((3 - (i % 4)) * 2)
			got := Cell((out[i/4] >> shift) & 0x03)
			want := b.Get(lx, ly)
			if got != want {
				t.Fatalf("PackChunk mismatch at %d,%d: got=%d want=%d", lx, ly, got, want)
			}
		}
	}
}

func TestLockReadBoxSpansChunks(t *testing.T) {
	b := NewBoard()
	ids := b.LockReadBox(60, 60, MaxFlipDistance)
	defer b.UnlockChunksRead(ids)
	if len(ids) < 4 {
		t.Fatalf("expected at least 4 chunks for box at (60,60) ± 50, got %d", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("chunk ids not strictly increasing: %v", ids)
		}
	}
}

func TestSeedCounts(t *testing.T) {
	b := NewBoard()
	b.Seed()

	var black, white int
	for y := 0; y < BoardSize; y++ {
		for x := 0; x < BoardSize; x++ {
			switch b.Get(x, y) {
			case CellBlack:
				black++
			case CellWhite:
				white++
			}
		}
	}

	wantEach := ChunksPerAxis * ChunksPerAxis * 2
	if black != wantEach || white != wantEach {
		t.Fatalf("seed counts mismatch: black=%d white=%d wantEach=%d", black, white, wantEach)
	}
}
