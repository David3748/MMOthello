package board

import "sync"

const (
	BoardSize      = 1000
	ChunkSize      = 50
	ChunksPerAxis  = BoardSize / ChunkSize
	TotalChunks    = ChunksPerAxis * ChunksPerAxis
	CellsPerBoard  = BoardSize * BoardSize
	PackedByteSize = CellsPerBoard / 4

	MaxFlipDistance = 50
)

type Cell uint8

const (
	CellEmpty Cell = iota
	CellBlack
	CellWhite
	CellReserved
)

type Coord struct {
	X uint16
	Y uint16
}

type ChunkMeta struct {
	mu          sync.RWMutex
	version     uint64
	subscribers map[uint64]struct{}
	blackCount  uint32
	whiteCount  uint32
}

type Board struct {
	cells  []byte
	chunks [TotalChunks]ChunkMeta
}

func NewBoard() *Board {
	b := &Board{
		cells: make([]byte, PackedByteSize),
	}
	for i := range b.chunks {
		b.chunks[i].subscribers = make(map[uint64]struct{})
	}
	return b
}

func Opponent(team Cell) Cell {
	switch team {
	case CellBlack:
		return CellWhite
	case CellWhite:
		return CellBlack
	default:
		return CellEmpty
	}
}
