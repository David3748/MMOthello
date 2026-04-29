package game

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mmothello/server/internal/board"
)

func TestPlaceCooldown(t *testing.T) {
	b := board.NewBoard()
	g := New(b)

	now := time.Unix(0, 0)
	g.SetNowFn(func() time.Time { return now })

	sess := &Session{ID: 1, Team: board.CellBlack}
	_ = b.Set(11, 10, board.CellWhite)
	_ = b.Set(12, 10, board.CellBlack)

	if _, err := g.Place(sess, 10, 10); err != nil {
		t.Fatalf("first place failed: %v", err)
	}

	now = now.Add(1 * time.Second)
	_ = b.Set(21, 20, board.CellWhite)
	_ = b.Set(22, 20, board.CellBlack)
	if _, err := g.Place(sess, 20, 20); !errors.Is(err, ErrCooldown) {
		t.Fatalf("expected ErrCooldown, got %v", err)
	}
}

func TestConcurrentSameCellOnlyOneWins(t *testing.T) {
	b := board.NewBoard()
	g := New(b)
	g.SetNowFn(func() time.Time { return time.Unix(0, 0).Add(10 * time.Second) })

	// Legal move at (10,10) for both players if white is bracketed by black.
	_ = b.Set(11, 10, board.CellWhite)
	_ = b.Set(12, 10, board.CellBlack)

	s1 := &Session{ID: 1, Team: board.CellBlack}
	s2 := &Session{ID: 2, Team: board.CellBlack}

	var wg sync.WaitGroup
	var successes atomic.Int32

	tryPlace := func(s *Session) {
		defer wg.Done()
		_, err := g.Place(s, 10, 10)
		if err == nil {
			successes.Add(1)
		}
	}

	wg.Add(2)
	go tryPlace(s1)
	go tryPlace(s2)
	wg.Wait()

	if successes.Load() != 1 {
		t.Fatalf("expected exactly one success, got %d", successes.Load())
	}
}

func TestErrorCodeMapping(t *testing.T) {
	if got := ErrorCode(ErrCooldown); got != ErrCodeCooldown {
		t.Fatalf("cooldown code mismatch: got=%d", got)
	}
	if got := ErrorCode(ErrOccupied); got != ErrCodeOccupied {
		t.Fatalf("occupied code mismatch: got=%d", got)
	}
	if got := ErrorCode(ErrNoFlips); got != ErrCodeNoFlips {
		t.Fatalf("no flips code mismatch: got=%d", got)
	}
	if got := ErrorCode(ErrOutOfBounds); got != ErrCodeOutOfBounds {
		t.Fatalf("out of bounds code mismatch: got=%d", got)
	}
	if got := ErrorCode(errors.New("unknown")); got != 0 {
		t.Fatalf("unknown code mismatch: got=%d", got)
	}
}
