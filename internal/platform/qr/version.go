package qr

import "errors"

// ErrTooLong is returned when the payload does not fit the largest version
// this package builds.
var ErrTooLong = errors.New("qr: payload too long")

// Only error-correction level M is built, and only versions 1 to 10.
//
// M recovers about 15% of a damaged symbol, which is the usual choice for
// something printed on a screen and photographed. Version 10 holds 216 data
// codewords — an otpauth:// URI runs to roughly 150 bytes, so the ceiling is
// not close, and every version above 10 needs a wider character-count field
// and a second alignment-pattern row for no benefit here.
type version struct {
	// ecPerBlock is error-correction codewords per block; every block in a
	// version has the same number.
	ecPerBlock int
	// Blocks come in two groups. The second group's blocks each hold one
	// more data codeword than the first, and a version may have none.
	group1Blocks, group1Data int
	group2Blocks, group2Data int
	// align are the centre coordinates of the alignment patterns. The
	// pattern is omitted where it would collide with a finder.
	align []int
}

var versions = [11]version{
	{}, // index 0 is unused; versions are 1-based
	{ecPerBlock: 10, group1Blocks: 1, group1Data: 16},
	{ecPerBlock: 16, group1Blocks: 1, group1Data: 28, align: []int{6, 18}},
	{ecPerBlock: 26, group1Blocks: 1, group1Data: 44, align: []int{6, 22}},
	{ecPerBlock: 18, group1Blocks: 2, group1Data: 32, align: []int{6, 26}},
	{ecPerBlock: 24, group1Blocks: 2, group1Data: 43, align: []int{6, 30}},
	{ecPerBlock: 16, group1Blocks: 4, group1Data: 27, align: []int{6, 34}},
	{ecPerBlock: 18, group1Blocks: 4, group1Data: 31, align: []int{6, 22, 38}},
	{ecPerBlock: 22, group1Blocks: 2, group1Data: 38, group2Blocks: 2, group2Data: 39, align: []int{6, 24, 42}},
	{ecPerBlock: 22, group1Blocks: 3, group1Data: 36, group2Blocks: 2, group2Data: 37, align: []int{6, 26, 46}},
	{ecPerBlock: 26, group1Blocks: 4, group1Data: 43, group2Blocks: 1, group2Data: 44, align: []int{6, 28, 50}},
}

// dataCodewords is how many data codewords this version holds in total.
func (v version) dataCodewords() int {
	return v.group1Blocks*v.group1Data + v.group2Blocks*v.group2Data
}

func (v version) blocks() int { return v.group1Blocks + v.group2Blocks }

// totalCodewords is data plus error correction, which is what the symbol has
// room for.
func (v version) totalCodewords() int {
	return v.dataCodewords() + v.blocks()*v.ecPerBlock
}

// size is the width of the symbol in modules, quiet zone excluded.
func size(ver int) int { return 17 + 4*ver }

// countBits is the width of the character-count field in byte mode. It steps
// up at version 10, which is why this package stops there: one rule, no
// branch that only fires on payloads nobody sends.
func countBits(ver int) int {
	if ver < 10 {
		return 8
	}
	return 16
}

// smallestVersion picks the first version that holds n bytes in byte mode.
func smallestVersion(n int) (int, error) {
	for ver := 1; ver <= 10; ver++ {
		// 4 bits of mode indicator plus the count field, rounded up to whole
		// codewords, has to leave room for the payload.
		capacity := versions[ver].dataCodewords() - 1 - countBits(ver)/8
		if n <= capacity {
			return ver, nil
		}
	}
	return 0, ErrTooLong
}
