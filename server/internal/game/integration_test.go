package game

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mmothello/server/internal/board"
)

// TestManyConcurrentPlacementsBoardConsistent exercises Place() from many
// goroutines, each on its own session, simulating concurrent users on
// different valid frontiers in the seeded board. The invariant: aggregate
// per-chunk counts match what we manually re-tally afterward.
func TestManyConcurrentPlacementsBoardConsistent(t *testing.T) {
	b := board.NewBoard()
	b.Seed()
	g := New(b)
	g.SetNowFn(func() time.Time { return time.Unix(10000, 0) })

	// Each worker does one placement on a fresh session so we don't need to
	// advance the clock from multiple goroutines.
	const numWorkers = 100
	var wg sync.WaitGroup
	var ok atomic.Int64
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cx := idx % 20
			cy := (idx / 20) % 20
			x0 := cx*50 + 24
			y0 := cy*50 + 24
			candidates := [][2]int{
				{x0 - 1, y0}, {x0 + 2, y0}, {x0, y0 - 1}, {x0, y0 + 2},
			}
			team := board.CellBlack
			if idx%2 == 0 {
				team = board.CellWhite
			}
			sess := &Session{ID: uint64(idx + 1), Team: team}
			pick := candidates[idx%4]
			if _, err := g.Place(sess, pick[0], pick[1]); err == nil {
				ok.Add(1)
			}
		}(w)
	}
	wg.Wait()

	// Re-tally and compare against per-chunk counts.
	var black, white uint32
	for y := 0; y < board.BoardSize; y++ {
		for x := 0; x < board.BoardSize; x++ {
			switch b.Get(x, y) {
			case board.CellBlack:
				black++
			case board.CellWhite:
				white++
			}
		}
	}
	bk, wt, em := b.Score()
	if uint64(black) != bk || uint64(white) != wt {
		t.Fatalf("score mismatch: tallied=(%d,%d) Score=(%d,%d,%d)", black, white, bk, wt, em)
	}
	if uint64(board.CellsPerBoard)-uint64(black)-uint64(white) != em {
		t.Fatalf("empty count drift")
	}
}

func TestPlaceFlipsAreApplied(t *testing.T) {
	b := board.NewBoard()
	g := New(b)
	g.SetNowFn(func() time.Time { return time.Unix(0, 0) })

	// Construct: black at (10,10), opponents (white) at (11,10) and (12,10),
	// then black places at (13,10) which should flip both whites to black.
	_ = b.Set(10, 10, board.CellBlack)
	_ = b.Set(11, 10, board.CellWhite)
	_ = b.Set(12, 10, board.CellWhite)

	sess := &Session{ID: 1, Team: board.CellBlack}
	flips, err := g.Place(sess, 13, 10)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if len(flips) != 2 {
		t.Fatalf("flips=%d want 2", len(flips))
	}
	if b.Get(11, 10) != board.CellBlack || b.Get(12, 10) != board.CellBlack {
		t.Fatalf("flips not applied: %d %d", b.Get(11, 10), b.Get(12, 10))
	}
	if b.Get(13, 10) != board.CellBlack {
		t.Fatalf("placed cell not set")
	}
}
