package repository

type RoutePolicyInput struct {
	Priority      int
	Effect        string
	PathPattern   string
	HostPattern   string
	Methods       []string
	PermissionID  string // "" for effects without a permission
	ScopeBindings []byte
}
