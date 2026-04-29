package board

import "testing"

func TestComputeFlipsEachDirection(t *testing.T) {
	type tc struct {
		name string
		dx   int
		dy   int
	}
	cases := []tc{
		{"nw", -1, -1}, {"n", 0, -1}, {"ne", 1, -1},
		{"w", -1, 0}, {"e", 1, 0},
		{"sw", -1, 1}, {"s", 0, 1}, {"se", 1, 1},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBoard()
			x, y := 20, 20
			_ = b.Set(x+tt.dx, y+tt.dy, CellWhite)
			_ = b.Set(x+2*tt.dx, y+2*tt.dy, CellBlack)

			flips := ComputeFlips(b, x, y, CellBlack)
			if len(flips) != 1 {
				t.Fatalf("got %d flips, want 1", len(flips))
			}
			if int(flips[0].X) != x+tt.dx || int(flips[0].Y) != y+tt.dy {
				t.Fatalf("unexpected flip coord %+v", flips[0])
			}
		})
	}
}

func TestComputeFlipsMultiDirection(t *testing.T) {
	b := NewBoard()
	x, y := 50, 50
	_ = b.Set(x+1, y, CellWhite)
	_ = b.Set(x+2, y, CellBlack)
	_ = b.Set(x, y+1, CellWhite)
	_ = b.Set(x, y+2, CellBlack)

	flips := ComputeFlips(b, x, y, CellBlack)
	if len(flips) != 2 {
		t.Fatalf("got %d flips, want 2", len(flips))
	}
}

func TestComputeFlipsNoFlipCases(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Board)
	}{
		{
			name: "empty terminator",
			setup: func(b *Board) {
				_ = b.Set(11, 10, CellWhite)
				_ = b.Set(12, 10, CellEmpty)
			},
		},
		{
			name: "edge terminator",
			setup: func(b *Board) {
				_ = b.Set(998, 10, CellWhite)
			},
		},
		{
			name: "lone opponent",
			setup: func(b *Board) {
				_ = b.Set(11, 10, CellWhite)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBoard()
			tt.setup(b)
			if flips := ComputeFlips(b, 10, 10, CellBlack); len(flips) != 0 {
				t.Fatalf("got %d flips, want 0", len(flips))
			}
		})
	}
}

func TestComputeFlipsRespectsCap(t *testing.T) {
	b := NewBoard()
	x, y := 100, 100
	for i := 1; i <= MaxFlipDistance; i++ {
		_ = b.Set(x+i, y, CellWhite)
	}
	_ = b.Set(x+MaxFlipDistance+1, y, CellBlack)

	if flips := ComputeFlips(b, x, y, CellBlack); len(flips) != 0 {
		t.Fatalf("got %d flips, want 0 due to cap", len(flips))
	}
}

func TestComputeFlipsCornerAndEdge(t *testing.T) {
	b := NewBoard()
	_ = b.Set(1, 0, CellWhite)
	_ = b.Set(2, 0, CellBlack)
	_ = b.Set(0, 1, CellWhite)
	_ = b.Set(0, 2, CellBlack)

	flips := ComputeFlips(b, 0, 0, CellBlack)
	if len(flips) != 2 {
		t.Fatalf("got %d flips, want 2", len(flips))
	}
}
