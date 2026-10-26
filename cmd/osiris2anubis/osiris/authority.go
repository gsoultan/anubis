package osiris

// Authority is one iam.authorities row: this policy confers this rule.
type Authority struct {
	PolicyID string
	RuleID   string
}
