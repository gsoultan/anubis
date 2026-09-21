package qr

import (
	"bytes"
	"testing"
)

// The generator polynomial coefficients are published for every block size a
// QR code uses. Getting these wrong produces a code that looks right and
// scans as corrupt, so they are checked against the table rather than
// against this implementation's own output.
func TestGeneratorPolynomialsMatchTheSpec(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want []byte
	}{
		{7, []byte{1, 127, 122, 154, 164, 11, 68, 117}},
		{10, []byte{1, 216, 194, 159, 111, 199, 94, 95, 113, 157, 193}},
		{13, []byte{1, 137, 73, 227, 17, 177, 17, 52, 13, 46, 43, 83, 132, 120}},
		{16, []byte{1, 59, 13, 104, 189, 68, 209, 30, 8, 163, 65, 41, 229, 98, 50, 36, 59}},
	} {
		if got := rsGenerator(tc.n); !bytes.Equal(got, tc.want) {
			t.Errorf("rsGenerator(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

// The worked example from the QR specification: the sixteen data codewords
// for "01234567" at version 1-M, and the ten error-correction codewords they
// produce.
//
// This is the load-bearing test in the package. Reed-Solomon is the part
// that cannot be checked by looking at the output — a wrong remainder is
// still a plausible-looking square — and it is verified here against a
// vector this implementation had no hand in producing.
func TestReedSolomonMatchesTheSpecExample(t *testing.T) {
	data := []byte{
		0x10, 0x20, 0x0C, 0x56, 0x61, 0x80, 0xEC, 0x11,
		0xEC, 0x11, 0xEC, 0x11, 0xEC, 0x11, 0xEC, 0x11,
	}
	want := []byte{0xA5, 0x24, 0xD4, 0xC1, 0xED, 0x36, 0xC7, 0x87, 0x2C, 0x55}
	if got := rsEncode(data, 10); !bytes.Equal(got, want) {
		t.Fatalf("rsEncode = % X, want % X", got, want)
	}
}

// a^255 = 1 closes the field, and every non-zero element has an inverse.
func TestFieldClosesOnItself(t *testing.T) {
	if gfExp[255] != 1 {
		t.Fatalf("a^255 = %d, want 1", gfExp[255])
	}
	for a := 1; a < 256; a++ {
		var found bool
		for b := 1; b < 256; b++ {
			if gfMul(byte(a), byte(b)) == 1 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%d has no multiplicative inverse", a)
		}
	}
}
