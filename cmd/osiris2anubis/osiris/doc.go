// Package osiris is the shape of the Osiris IAM tables the export reads:
// its rule catalog (rule ← policy ← group, and the feature/module/app the
// policy hangs off) and its people (user, placement, group membership).
//
// Plain rows, no behaviour. The conversion lives in package convert; the
// reading lives in package pgsource; this package is what the two agree on,
// so the converter can be tested on fixtures that never touch a database.
package osiris
