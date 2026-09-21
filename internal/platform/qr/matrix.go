package qr

// Code is a rendered symbol: Size modules square, quiet zone excluded.
type Code struct {
	Size    int
	Version int
	modules []bool
}

// Dark reports whether the module at (x, y) is dark. Out-of-range is light,
// so a caller drawing a quiet zone needs no bounds logic.
func (c *Code) Dark(x, y int) bool {
	if x < 0 || y < 0 || x >= c.Size || y >= c.Size {
		return false
	}
	return c.modules[y*c.Size+x]
}

type grid struct {
	size     int
	dark     []bool
	reserved []bool // function patterns: never carry data, never masked
}

func newGrid(size int) *grid {
	return &grid{size: size, dark: make([]bool, size*size), reserved: make([]bool, size*size)}
}

func (g *grid) at(x, y int) int { return y*g.size + x }

func (g *grid) set(x, y int, dark, reserved bool) {
	g.dark[g.at(x, y)] = dark
	if reserved {
		g.reserved[g.at(x, y)] = true
	}
}

// finder draws one 7x7 finder pattern and the separator around it. The
// separator is drawn as reserved light modules, including where it falls
// outside the symbol, which the bounds check discards.
func (g *grid) finder(cx, cy int) {
	for dy := -1; dy <= 7; dy++ {
		for dx := -1; dx <= 7; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= g.size || y >= g.size {
				continue
			}
			inRing := dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6 &&
				(dx == 0 || dx == 6 || dy == 0 || dy == 6)
			inCore := dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4
			g.set(x, y, inRing || inCore, true)
		}
	}
}

// alignment draws the 5x5 pattern centred on (cx, cy).
func (g *grid) alignment(cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			ring := dx == -2 || dx == 2 || dy == -2 || dy == 2
			g.set(cx+dx, cy+dy, ring || (dx == 0 && dy == 0), true)
		}
	}
}

// functionPatterns lays down everything that is not data: finders, timing,
// alignment, the dark module, and the areas format and version information
// are written into later.
func (g *grid) functionPatterns(ver int) {
	s := g.size
	g.finder(0, 0)
	g.finder(s-7, 0)
	g.finder(0, s-7)

	// Timing patterns run between the finders on row and column 6.
	for i := 8; i < s-8; i++ {
		on := i%2 == 0
		g.set(i, 6, on, true)
		g.set(6, i, on, true)
	}

	// Alignment patterns at every pair of centres, except the three that
	// would land on a finder.
	centres := versions[ver].align
	for _, cy := range centres {
		for _, cx := range centres {
			if (cx == 6 && cy == 6) || (cx == 6 && cy == s-7) || (cx == s-7 && cy == 6) {
				continue
			}
			g.alignment(cx, cy)
		}
	}

	// Reserve the format information areas around the finders.
	for i := 0; i <= 8; i++ {
		if i != 6 {
			g.set(i, 8, false, true)
			g.set(8, i, false, true)
		}
	}
	for i := 0; i < 8; i++ {
		g.set(s-1-i, 8, false, true)
	}
	for i := 0; i < 7; i++ {
		g.set(8, s-1-i, false, true)
	}

	// The dark module, after the reservations rather than before: it sits at
	// the end of the format information's column and the loop above would
	// otherwise clear it.
	g.set(8, s-8, true, true)

	// Version information, for versions 7 and up.
	if ver >= 7 {
		for i := 0; i < 18; i++ {
			g.set(i/3, s-11+i%3, false, true)
			g.set(s-11+i%3, i/3, false, true)
		}
	}
}

// placeData walks the symbol in the order the standard prescribes: two
// columns at a time from the right, alternating upward and downward, and
// skipping the vertical timing column entirely.
func (g *grid) placeData(codewords []byte) {
	bit := 0
	next := func() bool {
		if bit >= len(codewords)*8 {
			return false // remainder bits are light
		}
		on := codewords[bit/8]&(0x80>>uint(bit%8)) != 0
		bit++
		return on
	}
	dir := -1
	row := g.size - 1
	for col := g.size - 1; col > 0; col -= 2 {
		if col == 6 {
			col-- // column 6 is timing; the pair steps left around it
		}
		for {
			for c := 0; c < 2; c++ {
				x := col - c
				if !g.reserved[g.at(x, row)] {
					g.dark[g.at(x, row)] = next()
				}
			}
			row += dir
			if row < 0 || row >= g.size {
				row -= dir
				dir = -dir
				break
			}
		}
	}
}
