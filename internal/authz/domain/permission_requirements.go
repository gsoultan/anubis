package authzdomain

import "time"

// PermissionRequirements are the step-up constraints a permission declares
// (permissions.requires_amr, permissions.max_auth_age). One central
// definition of "sensitive"; applications get step-up for free.
type PermissionRequirements struct {
	RequiresAMR []string
	MaxAuthAge  time.Duration // 0 = no recency requirement
}
