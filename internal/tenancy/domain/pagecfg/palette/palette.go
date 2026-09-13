// Package palette is sRGB colour arithmetic: how light a colour is, and what
// sits halfway between two of them.
//
// It lives apart from pagecfg because it knows nothing about pages. pagecfg
// decides that a dark background needs a lifted card; this decides what "dark"
// and "lifted" mean, and it would answer the same for a chart or a badge.
//
// Everything is computed here rather than handed to CSS color-mix(), because
// the page this serves is the one an old phone has to render: color-mix is
// Safari 16.2 and newer, and a login screen that needs a current browser is a
// login screen somebody cannot use.
package palette

import (
	"fmt"
	"math"
)

// DarkThreshold is where white text overtakes black text on a background:
// below this luminance, 1.05/(L+0.05) beats (L+0.05)/0.05. Not a taste
// setting — it is where the two contrast ratios cross.
const DarkThreshold = 0.179

// Luminance is WCAG relative luminance, 0 (black) to 1 (white). An
// unparseable colour reports white, so a typo keeps a page looking the way it
// always has rather than flipping it to dark.
func Luminance(hex string) float64 {
	r, g, b, ok := Parse(hex)
	if !ok {
		return 1
	}
	lin := func(c float64) float64 {
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// Dark reports whether white text belongs on this colour.
func Dark(hex string) bool { return Luminance(hex) < DarkThreshold }

// Contrast is the WCAG ratio between two colours, 1 (identical) to 21
// (black on white). 4.5 is the threshold for body text.
func Contrast(a, b string) float64 {
	la, lb := Luminance(a), Luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Parse accepts #rgb and #rrggbb — the two forms pagecfg validates — and
// returns each channel as 0..1.
func Parse(hex string) (r, g, b float64, ok bool) {
	if len(hex) == 4 {
		hex = "#" + string([]byte{hex[1], hex[1], hex[2], hex[2], hex[3], hex[3]})
	}
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, false
	}
	var v [3]float64
	for i := 0; i < 3; i++ {
		hi, ok1 := nibble(hex[1+i*2])
		lo, ok2 := nibble(hex[2+i*2])
		if !ok1 || !ok2 {
			return 0, 0, 0, false
		}
		v[i] = float64(hi*16+lo) / 255
	}
	return v[0], v[1], v[2], true
}

func nibble(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

// Mix blends a toward b by t (0..1) and returns #rrggbb. An unparseable
// input returns a unchanged.
func Mix(a, b string, t float64) string {
	ar, ag, ab, ok1 := Parse(a)
	br, bg, bb, ok2 := Parse(b)
	if !ok1 || !ok2 {
		return a
	}
	ch := func(x, y float64) int {
		v := int((x + (y-x)*t) * 255)
		return min(max(v, 0), 255)
	}
	return fmt.Sprintf("#%02x%02x%02x", ch(ar, br), ch(ag, bg), ch(ab, bb))
}
