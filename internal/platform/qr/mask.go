package qr

// The eight data masks. A symbol is unreadable if large areas come out the
// same colour or if something in the data happens to look like a finder, so
// all eight are applied and the least bad is kept.
var masks = [8]func(x, y int) bool{
	func(x, y int) bool { return (y+x)%2 == 0 },
	func(x, y int) bool { return y%2 == 0 },
	func(x, y int) bool { return x%3 == 0 },
	func(x, y int) bool { return (y+x)%3 == 0 },
	func(x, y int) bool { return (y/2+x/3)%2 == 0 },
	func(x, y int) bool { return (y*x)%2+(y*x)%3 == 0 },
	func(x, y int) bool { return ((y*x)%2+(y*x)%3)%2 == 0 },
	func(x, y int) bool { return ((y+x)%2+(y*x)%3)%2 == 0 },
}

// applyMask flips every data module the mask selects. Function patterns are
// never masked, which is why they are marked reserved.
func (g *grid) applyMask(m int) {
	for y := 0; y < g.size; y++ {
		for x := 0; x < g.size; x++ {
			if !g.reserved[g.at(x, y)] && masks[m](x, y) {
				g.dark[g.at(x, y)] = !g.dark[g.at(x, y)]
			}
		}
	}
}

// penalty scores a masked symbol by the standard's four rules. Lower is
// better. A wrong score picks a worse mask rather than an invalid one, so
// this affects how easily a scanner reads the symbol and not whether it can.
func (g *grid) penalty() int {
	s, total := g.size, 0
	dark := 0

	// Rule 1: runs of five or more same-coloured modules in a line.
	run := func(get func(i int) bool) {
		length, last := 1, get(0)
		for i := 1; i < s; i++ {
			if c := get(i); c == last {
				length++
			} else {
				if length >= 5 {
					total += 3 + (length - 5)
				}
				length, last = 1, c
			}
		}
		if length >= 5 {
			total += 3 + (length - 5)
		}
	}
	for i := 0; i < s; i++ {
		row, col := i, i
		run(func(j int) bool { return g.dark[g.at(j, row)] })
		run(func(j int) bool { return g.dark[g.at(col, j)] })
	}

	// Rule 2: every 2x2 block of one colour.
	for y := 0; y < s-1; y++ {
		for x := 0; x < s-1; x++ {
			c := g.dark[g.at(x, y)]
			if c == g.dark[g.at(x+1, y)] && c == g.dark[g.at(x, y+1)] && c == g.dark[g.at(x+1, y+1)] {
				total += 3
			}
		}
	}

	// Rule 3: the finder-like sequence 1011101 with four light modules on
	// either side, in any row or column. A scanner hunting for finders must
	// not find one in the data.
	pattern := []bool{true, false, true, true, true, false, true}
	looksLikeFinder := func(get func(i int) bool, at int) bool {
		for i, want := range pattern {
			if get(at+i) != want {
				return false
			}
		}
		clear := func(from, n int) bool {
			for i := 0; i < n; i++ {
				j := from + i
				if j < 0 || j >= s {
					continue // outside the symbol counts as light
				}
				if get(j) {
					return false
				}
			}
			return true
		}
		return clear(at-4, 4) || clear(at+7, 4)
	}
	for i := 0; i < s; i++ {
		row, col := i, i
		byRow := func(j int) bool { return j >= 0 && j < s && g.dark[g.at(j, row)] }
		byCol := func(j int) bool { return j >= 0 && j < s && g.dark[g.at(col, j)] }
		for j := 0; j <= s-7; j++ {
			if looksLikeFinder(byRow, j) {
				total += 40
			}
			if looksLikeFinder(byCol, j) {
				total += 40
			}
		}
	}

	// Rule 4: how far the proportion of dark modules strays from half.
	for _, d := range g.dark {
		if d {
			dark++
		}
	}
	percent := dark * 100 / (s * s)
	deviation := percent - 50
	if deviation < 0 {
		deviation = -deviation
	}
	total += (deviation / 5) * 10
	return total
}

// formatBits is the 15-bit format information: two bits of error-correction
// level, three of mask, and a BCH(15,5) check, all XORed with a fixed mask so
// the all-zero case is not all-light.
func formatBits(mask int) int {
	const eccM = 0b00
	data := eccM<<3 | mask
	bch := data << 10
	for i := 14; i >= 10; i-- {
		if bch&(1<<uint(i)) != 0 {
			bch ^= 0b10100110111 << uint(i-10)
		}
	}
	return (data<<10 | bch) ^ 0b101010000010010
}

// versionBits is the 18-bit version information carried by versions 7 and up:
// six bits of version and a BCH(18,6) check.
func versionBits(ver int) int {
	bch := ver << 12
	for i := 17; i >= 12; i-- {
		if bch&(1<<uint(i)) != 0 {
			bch ^= 0b1111100100101 << uint(i-12)
		}
	}
	return ver<<12 | bch
}

// writeFormat writes both copies of the format information. Two copies
// because losing the corner that says which mask was used would cost the
// whole symbol.
func (g *grid) writeFormat(mask int) {
	bits := formatBits(mask)
	on := func(i int) bool { return bits&(1<<uint(i)) != 0 }
	s := g.size
	for i := 0; i <= 5; i++ {
		g.dark[g.at(i, 8)] = on(i)
	}
	g.dark[g.at(7, 8)] = on(6)
	g.dark[g.at(8, 8)] = on(7)
	g.dark[g.at(8, 7)] = on(8)
	for i := 9; i <= 14; i++ {
		g.dark[g.at(8, 14-i)] = on(i)
	}
	// The second copy runs the other way round: the low bits along row 8 to
	// the right, the high bits down column 8 at the bottom. Writing these
	// transposed still produces a symbol, and the dark module at (8, s-8)
	// disappears under it — which is how this was caught.
	for i := 0; i <= 7; i++ {
		g.dark[g.at(s-1-i, 8)] = on(i)
	}
	for i := 8; i <= 14; i++ {
		g.dark[g.at(8, s-15+i)] = on(i)
	}
}

func (g *grid) writeVersion(ver int) {
	if ver < 7 {
		return
	}
	bits := versionBits(ver)
	s := g.size
	for i := 0; i < 18; i++ {
		on := bits&(1<<uint(i)) != 0
		g.dark[g.at(i/3, s-11+i%3)] = on
		g.dark[g.at(s-11+i%3, i/3)] = on
	}
}
