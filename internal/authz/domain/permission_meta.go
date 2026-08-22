package authzdomain

import "time"

type PermissionMeta struct {
	ID           string
	Key          string
	Risk         string
	MinAssurance int
	RequiresAMR  []string
	MaxAuthAge   time.Duration
	Deprecated   bool
}
