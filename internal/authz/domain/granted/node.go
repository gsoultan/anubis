package granted

// Node is one place a grant reaches on an axis. Inherit says whether
// everything inside it is reached too; Except lists the places the same
// grant carves out, each with its own reach. An exception belongs to the
// grant that named it: it narrows THIS node, not a node another grant
// reaches independently.
type Node struct {
	NodeID  string
	Inherit bool
	Except  []Node
}
