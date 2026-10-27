package insight

// HolderKinds counts one role's grants by what they reach on one kind of
// place: everything, a branch that grows inside itself, a fixed list, or
// nothing at all on a kind every grant must name. Full is the fixed lists
// that name every place under some parent by hand.
type HolderKinds struct {
	Total      int
	Everything int
	Branch     int
	Fixed      int
	Nothing    int
	Full       int
}
