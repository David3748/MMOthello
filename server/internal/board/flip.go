package board

var directions = [8][2]int{
	{-1, -1}, {0, -1}, {1, -1},
	{-1, 0}, /*self*/ {1, 0},
	{-1, 1}, {0, 1}, {1, 1},
}

func ComputeFlips(b *Board, x, y int, team Cell) []Coord {
	if !InBounds(x, y) || b.Get(x, y) != CellEmpty {
		return nil
	}
	opponent := Opponent(team)
	if opponent == CellEmpty {
		return nil
	}

	flips := make([]Coord, 0, 16)
	for _, d := range directions {
		dx, dy := d[0], d[1]
		run := make([]Coord, 0, 8)
		for step := 1; step <= MaxFlipDistance; step++ {
			nx := x + dx*step
			ny := y + dy*step
			if !InBounds(nx, ny) {
				run = run[:0]
				break
			}
			cell := b.Get(nx, ny)
			switch cell {
			case opponent:
				run = append(run, Coord{X: uint16(nx), Y: uint16(ny)})
			case team:
				if len(run) > 0 {
					flips = append(flips, run...)
				}
				run = run[:0]
				step = MaxFlipDistance + 1
			default:
				run = run[:0]
				step = MaxFlipDistance + 1
			}
		}
	}
	if len(flips) == 0 {
		return nil
	}
	return flips
}
