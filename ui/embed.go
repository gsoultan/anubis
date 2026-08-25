// Package consoleassets carries the built admin console so anubisd can serve
// it from its own origin — production is same-origin by design (the CORS
// knob is dev-only). The committed dist/index.html is a placeholder that
// says how to build the real one; scripts/build.sh and the Dockerfile
// overwrite it with the actual bundle before compiling the binary.
package consoleassets

import "embed"

//go:embed all:dist
var Dist embed.FS
