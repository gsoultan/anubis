package osiris

// Rule is one iam.rules row: what CheckRule(name) asks for.
type Rule struct {
	ID          string
	Name        string
	Description string
}
