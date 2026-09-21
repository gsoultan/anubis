package qr

// Arithmetic over GF(256) with the QR code's primitive polynomial
// x^8 + x^4 + x^3 + x^2 + 1 (0x11D), and the Reed-Solomon encoder built on
// it.
//
// This is here rather than pulled in because ADR-0002 allows no third-party
// libraries, and a QR code is the one thing standing between an operator and
// typing a 32-character secret by hand. It is about two hundred lines of
// table arithmetic that has not changed since 2000.

var (
	gfExp [512]byte // gfExp[i] = a^i, doubled so products need no modulo
	gfLog [256]byte // gfLog[a^i] = i
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

// gfMul multiplies in GF(256). Zero is special-cased because it has no
// logarithm.
func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

// rsGenerator is the generator polynomial for n error-correction codewords,
// (x - a^0)(x - a^1)...(x - a^(n-1)), coefficients highest power first.
func rsGenerator(n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		// Multiply g by (x - a^i); in GF(2^k) subtraction is XOR.
		next := make([]byte, len(g)+1)
		for j, c := range g {
			next[j] ^= c
			next[j+1] ^= gfMul(c, gfExp[i])
		}
		g = next
	}
	return g
}

// rsEncode returns the n error-correction codewords for data: the remainder
// of data*x^n divided by the generator polynomial.
func rsEncode(data []byte, n int) []byte {
	gen := rsGenerator(n)
	rem := make([]byte, len(data)+n)
	copy(rem, data)
	for i := 0; i < len(data); i++ {
		lead := rem[i]
		if lead == 0 {
			continue
		}
		for j, c := range gen {
			rem[i+j] ^= gfMul(c, lead)
		}
	}
	return rem[len(data):]
}
