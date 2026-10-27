package osiris

// Group is one iam.groups row, with the policies iam.permissions gives it.
type Group struct {
	ID          string
	Code        string
	Name        string
	Description string
	PolicyIDs   []string
}
