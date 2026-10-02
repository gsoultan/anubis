package provisioningapp

// membershipRef is what the import needs to know about a membership it adds
// people to: which one, and whether each row must say where.
type membershipRef struct {
	id         string
	anchorAxis string
}
