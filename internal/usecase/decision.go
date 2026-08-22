package usecase

// Decision is the authorize answer. On deny the failing axis is always named
// when scope was the reason; on step_up_required the caller learns exactly
// what authentication would satisfy the permission.
type Decision struct {
	Allow       bool
	Reason      string
	FailingAxis string
	Message     string
	RequiredAMR []string
	MaxAuthAge  string
	CurrentAMR  []string
	AuthAge     string
}
