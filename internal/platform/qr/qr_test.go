package qr

import (
	"fmt"
	"math/bits"
	"strings"
	"testing"
)

// Format information is a BCH(15,5) code with minimum distance 7, and the
// level-M mask-0 string is the XOR constant itself because both data fields
// are zero. Both are properties of the standard rather than of this code.
func TestFormatInformationMatchesTheStandard(t *testing.T) {
	if got := formatBits(0); got != 0b101010000010010 {
		t.Fatalf("level M mask 0 = %015b, want 101010000010010", got)
	}
	for a := 0; a < 8; a++ {
		for b := a + 1; b < 8; b++ {
			if d := bits.OnesCount(uint(formatBits(a) ^ formatBits(b))); d < 7 {
				t.Errorf("masks %d and %d differ in %d bits, want at least 7", a, b, d)
			}
		}
	}
}

// Version 7's version-information string is published; the code is
// BCH(18,6) with minimum distance 8.
func TestVersionInformationMatchesTheStandard(t *testing.T) {
	if got := versionBits(7); got != 0b000111110010010100 {
		t.Fatalf("version 7 = %018b, want 000111110010010100", got)
	}
	for a := 7; a <= 10; a++ {
		for b := a + 1; b <= 10; b++ {
			if d := bits.OnesCount(uint(versionBits(a) ^ versionBits(b))); d < 8 {
				t.Errorf("versions %d and %d differ in %d bits, want at least 8", a, b, d)
			}
		}
	}
}

// The capacity tables are the kind of thing that is wrong by one and still
// produces a symbol. Data plus error correction must exactly fill the
// version, and the published totals say what that is.
func TestCapacityTablesAreConsistent(t *testing.T) {
	totals := map[int]int{1: 26, 2: 44, 3: 70, 4: 100, 5: 134, 6: 172, 7: 196, 8: 242, 9: 292, 10: 346}
	for ver := 1; ver <= 10; ver++ {
		if got := versions[ver].totalCodewords(); got != totals[ver] {
			t.Errorf("version %d holds %d codewords, want %d", ver, got, totals[ver])
		}
		// A second group's blocks hold exactly one more codeword than the
		// first group's; anything else means the table was mistyped.
		v := versions[ver]
		if v.group2Blocks > 0 && v.group2Data != v.group1Data+1 {
			t.Errorf("version %d group 2 holds %d, want %d", ver, v.group2Data, v.group1Data+1)
		}
	}
}

// Fixed patterns land where the standard puts them. A symbol with a finder
// one module off is unreadable and looks entirely normal.
func TestFixedPatternsAreWhereTheyBelong(t *testing.T) {
	for _, ver := range []int{1, 6, 7, 10} {
		c, err := Encode(strings.Repeat("a", 10))
		if err != nil {
			t.Fatal(err)
		}
		g := newGrid(size(ver))
		g.functionPatterns(ver)
		s := g.size

		// Each finder is a dark ring with a dark core and a light gap.
		for _, o := range [][2]int{{0, 0}, {s - 7, 0}, {0, s - 7}} {
			for _, p := range [][3]int{{0, 0, 1}, {3, 3, 1}, {1, 1, 0}, {6, 6, 1}} {
				x, y, want := o[0]+p[0], o[1]+p[1], p[2] == 1
				if g.dark[g.at(x, y)] != want {
					t.Errorf("version %d finder at (%d,%d): module (%d,%d) wrong", ver, o[0], o[1], x, y)
				}
			}
		}
		// Timing patterns alternate, starting dark at module 8.
		for i := 8; i < s-8; i++ {
			if g.dark[g.at(i, 6)] != (i%2 == 0) || g.dark[g.at(6, i)] != (i%2 == 0) {
				t.Errorf("version %d timing pattern breaks at %d", ver, i)
			}
		}
		// The dark module is always dark.
		if !g.dark[g.at(8, s-8)] {
			t.Errorf("version %d has no dark module", ver)
		}
		if c.Size != size(c.Version) {
			t.Errorf("size %d does not match version %d", c.Size, c.Version)
		}
	}
}

// decode reads a symbol back: recover the mask from the format information,
// undo it, walk the data modules in placement order, de-interleave the
// blocks and strip the header.
//
// This exists to close the loop end to end. It cannot prove the placement
// matches the standard — a consistently wrong encoder and decoder would
// agree — which is why the fixed patterns, the format strings and the
// Reed-Solomon vectors are checked against the standard separately.
func decode(t *testing.T, c *Code) string {
	t.Helper()
	ver := c.Version
	v := versions[ver]
	g := &grid{size: c.Size, dark: make([]bool, len(c.modules)), reserved: make([]bool, len(c.modules))}
	copy(g.dark, c.modules)
	g.functionPatterns(ver) // marks reserved; overwrites function modules with the same values

	// Which mask was used, from the first copy of the format information.
	read := 0
	for i := 0; i <= 5; i++ {
		if c.Dark(i, 8) {
			read |= 1 << uint(i)
		}
	}
	if c.Dark(7, 8) {
		read |= 1 << 6
	}
	if c.Dark(8, 8) {
		read |= 1 << 7
	}
	if c.Dark(8, 7) {
		read |= 1 << 8
	}
	for i := 9; i <= 14; i++ {
		if c.Dark(8, 14-i) {
			read |= 1 << uint(i)
		}
	}
	mask := -1
	for m := 0; m < 8; m++ {
		if formatBits(m) == read {
			mask = m
		}
	}
	if mask < 0 {
		t.Fatalf("format information %015b matches no mask", read)
	}
	g.applyMask(mask)

	// Walk the same path placeData took.
	var raw []byte
	var cur byte
	nbits := 0
	push := func(on bool) {
		cur <<= 1
		if on {
			cur |= 1
		}
		nbits++
		if nbits == 8 {
			raw = append(raw, cur)
			cur, nbits = 0, 0
		}
	}
	dir, row := -1, g.size-1
	for col := g.size - 1; col > 0; col -= 2 {
		if col == 6 {
			col--
		}
		for {
			for k := 0; k < 2; k++ {
				x := col - k
				if !g.reserved[g.at(x, row)] {
					push(g.dark[g.at(x, row)])
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

	// De-interleave: the reverse of interleave's column-wise read.
	lengths := make([]int, 0, v.blocks())
	for i := 0; i < v.group1Blocks; i++ {
		lengths = append(lengths, v.group1Data)
	}
	for i := 0; i < v.group2Blocks; i++ {
		lengths = append(lengths, v.group2Data)
	}
	blocks := make([][]byte, len(lengths))
	at := 0
	longest := v.group1Data
	if v.group2Data > longest {
		longest = v.group2Data
	}
	for i := 0; i < longest; i++ {
		for b, n := range lengths {
			if i < n {
				blocks[b] = append(blocks[b], raw[at])
				at++
			}
		}
	}
	var data []byte
	for _, b := range blocks {
		data = append(data, b...)
	}

	// Strip the header: four bits of mode, then the character count.
	if data[0]>>4 != 0b0100 {
		t.Fatalf("mode indicator is %04b, want 0100", data[0]>>4)
	}
	r := &bitReader{data: data, pos: 4}
	n := r.read(countBits(ver))
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(r.read(8))
	}
	return string(out)
}

type bitReader struct {
	data []byte
	pos  int
}

func (r *bitReader) read(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		v <<= 1
		if r.data[r.pos/8]&(0x80>>uint(r.pos%8)) != 0 {
			v |= 1
		}
		r.pos++
	}
	return v
}

// Everything the enrolment page will ever hand it, read back out.
func TestSymbolsRoundTrip(t *testing.T) {
	cases := []string{
		"a",
		"otpauth://totp/Anubis:ops.riley?algorithm=SHA1&digits=6&issuer=Anubis&period=30&secret=JBSWY3DPEHPK3PXP",
		"otpauth://totp/Anubis%3Asomebody.with.a.long.name%40example.test?algorithm=SHA1&digits=6&issuer=Anubis&period=30&secret=MFRGGZDFMZTWQ2LKNNWG23TPOBYXE43UOJUW4ZY",
		strings.Repeat("x", 100),
		strings.Repeat("y", 213), // the version 10 ceiling: 1728 bits less a 20-bit header
	}
	for _, in := range cases {
		c, err := Encode(in)
		if err != nil {
			t.Fatalf("encode %d bytes: %v", len(in), err)
		}
		if got := decode(t, c); got != in {
			t.Fatalf("round trip of %d bytes came back as %d bytes:\n got %.60q\nwant %.60q",
				len(in), len(got), got, in)
		}
	}
}

func TestPayloadBeyondTheCeilingIsRefused(t *testing.T) {
	if _, err := Encode(strings.Repeat("z", 1000)); err != ErrTooLong {
		t.Fatalf("1000 bytes returned %v, want ErrTooLong", err)
	}
}

// The quiet zone is not decoration: without it a scanner has no edge to find.
func TestSVGCarriesAQuietZone(t *testing.T) {
	c, err := Encode("otpauth://totp/x?secret=ABC")
	if err != nil {
		t.Fatal(err)
	}
	svg := c.SVG(240)
	if want := fmt.Sprintf(`viewBox="0 0 %d %d"`, c.Size+8, c.Size+8); !strings.Contains(svg, want) {
		t.Fatalf("SVG has no four-module quiet zone; want %s", want)
	}
	if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatal("SVG is not a standalone element")
	}
}
