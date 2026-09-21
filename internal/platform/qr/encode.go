package qr

// bitWriter appends bits most-significant first, which is the order every
// field in a QR symbol is written in.
type bitWriter struct {
	bytes []byte
	nbits int
}

func (w *bitWriter) write(value, bits int) {
	for i := bits - 1; i >= 0; i-- {
		if w.nbits%8 == 0 {
			w.bytes = append(w.bytes, 0)
		}
		if value&(1<<uint(i)) != 0 {
			w.bytes[w.nbits/8] |= 0x80 >> uint(w.nbits%8)
		}
		w.nbits++
	}
}

// dataCodewords assembles the data half of the symbol: mode, length, the
// payload, a terminator, and the pad bytes that fill whatever is left.
func dataCodewords(payload []byte, ver int) []byte {
	v := versions[ver]
	w := &bitWriter{}
	w.write(0b0100, 4) // byte mode
	w.write(len(payload), countBits(ver))
	for _, b := range payload {
		w.write(int(b), 8)
	}

	// Terminator: up to four zero bits, fewer if the symbol is nearly full.
	capacity := v.dataCodewords() * 8
	if pad := capacity - w.nbits; pad > 4 {
		pad = 4
	} else if pad < 0 {
		pad = 0
	}
	w.write(0, min(4, capacity-w.nbits))

	// Pad to a whole codeword, then alternate the two pad bytes the standard
	// names until the data half is full.
	for w.nbits%8 != 0 {
		w.write(0, 1)
	}
	pads := []byte{0xEC, 0x11}
	for i := 0; len(w.bytes) < v.dataCodewords(); i++ {
		w.bytes = append(w.bytes, pads[i%2])
	}
	return w.bytes
}

// interleave splits the data into blocks, computes each block's error
// correction, and reads the whole lot back out column-wise.
//
// The interleaving is the point: a scratch across the symbol then damages a
// few codewords in every block rather than destroying one block outright,
// and Reed-Solomon corrects per block.
func interleave(data []byte, ver int) []byte {
	v := versions[ver]
	blocks := make([][]byte, 0, v.blocks())
	ecs := make([][]byte, 0, v.blocks())

	at := 0
	add := func(n, length int) {
		for i := 0; i < n; i++ {
			b := data[at : at+length]
			at += length
			blocks = append(blocks, b)
			ecs = append(ecs, rsEncode(b, v.ecPerBlock))
		}
	}
	add(v.group1Blocks, v.group1Data)
	add(v.group2Blocks, v.group2Data)

	out := make([]byte, 0, v.totalCodewords())
	longest := v.group1Data
	if v.group2Data > longest {
		longest = v.group2Data
	}
	for i := 0; i < longest; i++ {
		for _, b := range blocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	for i := 0; i < v.ecPerBlock; i++ {
		for _, e := range ecs {
			out = append(out, e[i])
		}
	}
	return out
}
