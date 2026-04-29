package board

import "sort"

func InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < BoardSize && y < BoardSize
}

func cellIndex(x, y int) int {
	return y*BoardSize + x
}

func (b *Board) Get(x, y int) Cell {
	if !InBounds(x, y) {
		return CellEmpty
	}
	i := cellIndex(x, y)
	byteIdx := i / 4
	shift := uint((3 - (i % 4)) * 2)
	return Cell((b.cells[byteIdx] >> shift) & 0x03)
}

func (b *Board) Set(x, y int, cell Cell) bool {
	if !InBounds(x, y) {
		return false
	}
	i := cellIndex(x, y)
	byteIdx := i / 4
	shift := uint((3 - (i % 4)) * 2)
	mask := byte(0x03 << shift)
	b.cells[byteIdx] = (b.cells[byteIdx] &^ mask) | (byte(cell&0x03) << shift)
	return true
}

func ChunkOf(x, y int) uint16 {
	cx := x / ChunkSize
	cy := y / ChunkSize
	return uint16(cy*ChunksPerAxis + cx)
}

func ChunkBounds(id uint16) (x0, y0, x1, y1 int) {
	cx := int(id) % ChunksPerAxis
	cy := int(id) / ChunksPerAxis
	x0 = cx * ChunkSize
	y0 = cy * ChunkSize
	x1 = x0 + ChunkSize - 1
	y1 = y0 + ChunkSize - 1
	return
}

func (b *Board) Seed() {
	for i := range b.cells {
		b.cells[i] = 0
	}
	for i := range b.chunks {
		ch := &b.chunks[i]
		ch.version = 0
		ch.blackCount = 0
		ch.whiteCount = 0
	}

	for cy := 0; cy < ChunksPerAxis; cy++ {
		for cx := 0; cx < ChunksPerAxis; cx++ {
			ox := cx*ChunkSize + 24
			oy := cy*ChunkSize + 24
			b.Set(ox, oy, CellWhite)
			b.Set(ox+1, oy, CellBlack)
			b.Set(ox, oy+1, CellBlack)
			b.Set(ox+1, oy+1, CellWhite)
		}
	}
	b.recountChunkCounts()
}

// RecountChunks rebuilds per-chunk black/white counts from cell data.
// Useful after restoring from a snapshot.
func (b *Board) RecountChunks() { b.recountChunkCounts() }

// Score returns aggregate (black, white, empty) counts across the board.
func (b *Board) Score() (black, white, empty uint64) {
	for id := uint16(0); id < TotalChunks; id++ {
		bk, wt := b.ChunkCounts(id)
		black += uint64(bk)
		white += uint64(wt)
	}
	empty = uint64(CellsPerBoard) - black - white
	return
}

func (b *Board) recountChunkCounts() {
	for i := range b.chunks {
		b.chunks[i].blackCount = 0
		b.chunks[i].whiteCount = 0
	}
	for y := 0; y < BoardSize; y++ {
		for x := 0; x < BoardSize; x++ {
			c := b.Get(x, y)
			id := ChunkOf(x, y)
			switch c {
			case CellBlack:
				b.chunks[id].blackCount++
			case CellWhite:
				b.chunks[id].whiteCount++
			}
		}
	}
}

func (b *Board) LockChunksWrite(ids []uint16) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		b.chunks[id].mu.Lock()
	}
}

func (b *Board) UnlockChunksWrite(ids []uint16) {
	for i := len(ids) - 1; i >= 0; i-- {
		b.chunks[ids[i]].mu.Unlock()
	}
}

func (b *Board) LockAllRead() {
	for i := range b.chunks {
		b.chunks[i].mu.RLock()
	}
}

func (b *Board) UnlockAllRead() {
	for i := len(b.chunks) - 1; i >= 0; i-- {
		b.chunks[i].mu.RUnlock()
	}
}

func (b *Board) ApplyCellChange(x, y int, next Cell) {
	prev := b.Get(x, y)
	if prev == next {
		return
	}
	id := ChunkOf(x, y)
	switch prev {
	case CellBlack:
		b.chunks[id].blackCount--
	case CellWhite:
		b.chunks[id].whiteCount--
	}
	switch next {
	case CellBlack:
		b.chunks[id].blackCount++
	case CellWhite:
		b.chunks[id].whiteCount++
	}
	b.Set(x, y, next)
}

func (b *Board) BumpVersion(id uint16) {
	b.chunks[id].version++
}

func (b *Board) ChunkVersion(id uint16) uint64 {
	return b.chunks[id].version
}

func (b *Board) ChunkCounts(id uint16) (black, white uint32) {
	ch := &b.chunks[id]
	ch.mu.RLock()
	black, white = ch.blackCount, ch.whiteCount
	ch.mu.RUnlock()
	return
}

// PackChunk copies a 50x50 chunk into a 625-byte buffer using the same
// 2-bit MSB-first packing as the global board, but indexed within the
// chunk: i = localY * ChunkSize + localX.
//
// Caller should hold the chunk's read lock.
func (b *Board) PackChunk(id uint16, out []byte) {
	if len(out) < 625 {
		panic("PackChunk: buffer too small")
	}
	for i := 0; i < 625; i++ {
		out[i] = 0
	}
	x0, y0, _, _ := ChunkBounds(id)
	for ly := 0; ly < ChunkSize; ly++ {
		for lx := 0; lx < ChunkSize; lx++ {
			c := b.Get(x0+lx, y0+ly)
			i := ly*ChunkSize + lx
			byteIdx := i / 4
			shift := uint((3 - (i % 4)) * 2)
			out[byteIdx] |= byte(c&0x03) << shift
		}
	}
}

// LockChunkRead acquires an RLock on a single chunk.
func (b *Board) LockChunkRead(id uint16)   { b.chunks[id].mu.RLock() }
func (b *Board) UnlockChunkRead(id uint16) { b.chunks[id].mu.RUnlock() }

// LockReadBox RLocks every chunk overlapping the bounding box around (x, y)
// extended by `radius` cells in each direction. Returns the locked ids in
// sorted order; caller must call UnlockChunksRead.
func (b *Board) LockReadBox(x, y, radius int) []uint16 {
	x0, y0 := x-radius, y-radius
	x1, y1 := x+radius, y+radius
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= BoardSize {
		x1 = BoardSize - 1
	}
	if y1 >= BoardSize {
		y1 = BoardSize - 1
	}
	cx0, cy0 := x0/ChunkSize, y0/ChunkSize
	cx1, cy1 := x1/ChunkSize, y1/ChunkSize
	ids := make([]uint16, 0, (cx1-cx0+1)*(cy1-cy0+1))
	for cy := cy0; cy <= cy1; cy++ {
		for cx := cx0; cx <= cx1; cx++ {
			ids = append(ids, uint16(cy*ChunksPerAxis+cx))
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		b.chunks[id].mu.RLock()
	}
	return ids
}

// UnlockChunksRead releases RLocks acquired by LockReadBox in reverse order.
func (b *Board) UnlockChunksRead(ids []uint16) {
	for i := len(ids) - 1; i >= 0; i-- {
		b.chunks[ids[i]].mu.RUnlock()
	}
}
