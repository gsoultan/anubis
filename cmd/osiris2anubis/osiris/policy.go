package osiris

// Policy is one iam.policies row: a named bundle of rules under a feature.
type Policy struct {
	ID          string
	Name        string
	Description string
	FeatureID   string
}
