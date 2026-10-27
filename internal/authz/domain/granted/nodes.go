package granted

import "time"

// Nodes is the answer to "where, on this axis, may this subject do
// this?" — the question an application asks ONCE per screen so it can
// filter a query, instead of asking Authorize once per row.
//
// Three shapes, because the three mean different things to a filter:
//
//   - Everywhere: some qualifying grant puts no limit on the axis. There is
//     no list; the application applies no filter.
//   - Nodes: the places qualifying grants name, each with its reach (the
//     place only, or everything inside it) and its exceptions. The
//     application turns these into the predicate its own data model needs.
//   - OwnRecords: a self-scoped grant qualifies. Rows the subject owns pass
//     regardless of place.
//
// Empty — none of the three — means nowhere: the screen shows its empty
// state, not every row.
//
// Everything here was true AsOf; it is read from the gate's snapshot, so a
// grant made a moment ago may be a few seconds away.
type Nodes struct {
	Everywhere bool
	OwnRecords bool
	Nodes      []Node
	AsOf       time.Time
}
