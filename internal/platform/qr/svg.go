package qr

import (
	"fmt"
	"strings"
)

// SVG renders the symbol as a standalone SVG element.
//
// One path of rectangles rather than one rect per module: a version 7 symbol
// is 2,025 modules, and 2,025 elements is a page a phone struggles to
// render. The viewBox carries the four-module quiet zone the standard
// requires — without it a scanner has nothing to find the symbol against.
func (c *Code) SVG(pixels int) string {
	const quiet = 4
	span := c.Size + quiet*2

	var path strings.Builder
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if !c.Dark(x, y) {
				continue
			}
			// Runs of dark modules become one rectangle.
			run := 1
			for x+run < c.Size && c.Dark(x+run, y) {
				run++
			}
			fmt.Fprintf(&path, "M%d %dh%dv1h-%dz", x+quiet, y+quiet, run, run)
			x += run - 1
		}
	}

	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" `+
			`viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" `+
			`aria-label="Enrolment QR code">`+
			`<rect width="%d" height="%d" fill="#fff"/>`+
			`<path d="%s" fill="#000"/></svg>`,
		pixels, pixels, span, span, span, span, path.String())
}
