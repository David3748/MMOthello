package game

import (
	"errors"
	"sort"
	"sync"
	"time"

	"mmothello/server/internal/board"
)

const Cooldown = 2 * time.Second

var (
	ErrOutOfBounds = errors.New("out of bounds")
	ErrCooldown    = errors.New("cooldown active")
	ErrOccupied    = errors.New("cell occupied")
	ErrNoFlips     = errors.New("no flips")
)

const (
	ErrCodeCooldown   uint8 = 1
	ErrCodeOccupied   uint8 = 2
	ErrCodeNoFlips    uint8 = 3
	ErrCodeOutOfBounds uint8 = 4
	ErrCodeAuth       uint8 = 5
	ErrCodeRateLimit  uint8 = 6
)

type Session struct {
	ID            uint64
	Team          board.Cell
	LastPlaceUnix int64
	placed        bool

	mu sync.Mutex
}

type Game struct {
	Board *board.Board
	nowFn func() time.Time
}

func New(b *board.Board) *Game {
	return &Game{
		Board: b,
		nowFn: time.Now,
	}
}

func (g *Game) SetNowFn(fn func() time.Time) {
	if fn == nil {
		g.nowFn = time.Now
		return
	}
	g.nowFn = fn
}

func (g *Game) Place(sess *Session, x, y int) ([]board.Coord, error) {
	if !board.InBounds(x, y) {
		return nil, ErrOutOfBounds
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	now := g.nowFn()
	if sess.placed {
		last := time.UnixMilli(sess.LastPlaceUnix)
		if now.Sub(last) < Cooldown {
			return nil, ErrCooldown
		}
	}

	// Read phase: only RLock the bounding-box of chunks that the flip scan
	// can reach. This avoids serializing distant placements through a single
	// global lock while still preventing torn reads.
	readIDs := g.Board.LockReadBox(x, y, board.MaxFlipDistance)
	flips := board.ComputeFlips(g.Board, x, y, sess.Team)
	occupied := g.Board.Get(x, y) != board.CellEmpty
	g.Board.UnlockChunksRead(readIDs)
	if occupied {
		return nil, ErrOccupied
	}
	if len(flips) == 0 {
		return nil, ErrNoFlips
	}

	touched := touchedChunks(x, y, flips)
	g.Board.LockChunksWrite(touched)
	defer g.Board.UnlockChunksWrite(touched)

	if g.Board.Get(x, y) != board.CellEmpty {
		return nil, ErrOccupied
	}
	flips = board.ComputeFlips(g.Board, x, y, sess.Team)
	if len(flips) == 0 {
		return nil, ErrNoFlips
	}

	g.Board.ApplyCellChange(x, y, sess.Team)
	for _, c := range flips {
		g.Board.ApplyCellChange(int(c.X), int(c.Y), sess.Team)
	}
	updated := touchedChunks(x, y, flips)
	for _, id := range updated {
		g.Board.BumpVersion(id)
	}
	sess.LastPlaceUnix = now.UnixMilli()
	sess.placed = true

	return flips, nil
}

func touchedChunks(x, y int, flips []board.Coord) []uint16 {
	seen := map[uint16]struct{}{
		board.ChunkOf(x, y): {},
	}
	for _, c := range flips {
		seen[board.ChunkOf(int(c.X), int(c.Y))] = struct{}{}
	}
	ids := make([]uint16, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func ErrorCode(err error) uint8 {
	switch {
	case errors.Is(err, ErrCooldown):
		return ErrCodeCooldown
	case errors.Is(err, ErrOccupied):
		return ErrCodeOccupied
	case errors.Is(err, ErrNoFlips):
		return ErrCodeNoFlips
	case errors.Is(err, ErrOutOfBounds):
		return ErrCodeOutOfBounds
	default:
		return 0
	}
}
