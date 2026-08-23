package identityapp

// SweepReport is what one retention pass did.
type SweepReport struct {
	Stamped    int64
	Anonymized int
	Shredded   int
}
