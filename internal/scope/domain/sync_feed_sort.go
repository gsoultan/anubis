package scopedomain

// SortFeedParentsFirst orders rows so every parent precedes its children —
// the contract scope_sync_apply relies on to resolve parent_ref.
//
// No SQL ORDER BY can do this: ordering by parent_ref puts the children of
// an alphabetically-early parent before that parent's own row. So the app
// tier sorts, and every source kind (http, db_query, db_table) gets the
// guarantee for free.
//
// Rows whose parent_ref names a row absent from the feed are treated as
// roots and kept — they may attach to a node that already exists in Anubis,
// and if they do not, the reconciler reports that row's own error rather
// than silently dropping it. Cycles cannot starve the output: whatever is
// still unplaced at the end is appended in input order.
func SortFeedParentsFirst(rows []SyncFeedRow) []SyncFeedRow {
	if len(rows) < 2 {
		return rows
	}
	index := make(map[string]int, len(rows))
	for i, r := range rows {
		if _, dup := index[r.Ref]; !dup {
			index[r.Ref] = i
		}
	}

	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)
	state := make([]int8, len(rows))
	out := make([]SyncFeedRow, 0, len(rows))

	// Iterative depth-first emit: parent chain first, then the row itself.
	var stack []int
	for start := range rows {
		if state[start] != unvisited {
			continue
		}
		stack = stack[:0]
		i := start
		for {
			if state[i] == done {
				break
			}
			parent, hasParent := index[rows[i].ParentRef]
			if rows[i].ParentRef == "" || !hasParent || parent == i {
				hasParent = false
			}
			if hasParent && state[parent] == unvisited {
				// Walk up before emitting; a cycle is cut when we meet a row
				// already on this stack.
				state[i] = inStack
				stack = append(stack, i)
				i = parent
				continue
			}
			out = append(out, rows[i])
			state[i] = done
			if len(stack) == 0 {
				break
			}
			i = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}
	}
	for i := range rows {
		if state[i] != done {
			out = append(out, rows[i])
		}
	}
	return out
}
