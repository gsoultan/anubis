package scopedomain

// ScopeAncestor is one step of the chain from an axis root down to a node.
// Depth counts hops from the node itself: the node is 0, its parent 1.
type ScopeAncestor struct {
	Node  ScopeNodeRecord
	Depth int
}
