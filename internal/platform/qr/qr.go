// Package qr builds QR code symbols.
//
// It exists because ADR-0002 allows no third-party libraries and the hosted
// enrolment page needs one: the alternative is asking somebody to copy a
// 32-character base32 secret into a phone by hand, which is the step people
// give up on.
//
// Byte mode, error-correction level M, versions 1 to 10 — enough for an
// otpauth:// URI with room to spare, and nothing beyond that. What it does
// NOT do is as deliberate as what it does: no kanji or alphanumeric modes, no
// structured append, no version above 10.
package qr

// Encode builds the smallest symbol that holds payload.
func Encode(payload string) (*Code, error) {
	ver, err := smallestVersion(len(payload))
	if err != nil {
		return nil, err
	}
	codewords := interleave(dataCodewords([]byte(payload), ver), ver)

	// Build once, then try each mask on a copy: masking is a XOR over the
	// data modules, so the cheapest correct way to compare eight of them is
	// to keep the unmasked symbol and re-apply.
	base := newGrid(size(ver))
	base.functionPatterns(ver)
	base.placeData(codewords)

	best, bestScore := -1, 1<<62
	for m := 0; m < 8; m++ {
		g := base.clone()
		g.applyMask(m)
		g.writeFormat(m)
		g.writeVersion(ver)
		if score := g.penalty(); score < bestScore {
			best, bestScore = m, score
		}
	}

	out := base.clone()
	out.applyMask(best)
	out.writeFormat(best)
	out.writeVersion(ver)
	return &Code{Size: out.size, Version: ver, modules: out.dark}, nil
}

func (g *grid) clone() *grid {
	c := &grid{size: g.size, dark: make([]bool, len(g.dark)), reserved: g.reserved}
	copy(c.dark, g.dark)
	return c
}
