package tenancydomain

type RoutePolicyRecord struct {
	ID            string
	AppSlug       string
	Priority      int
	Effect        string
	PathPattern   string
	HostPattern   string
	Methods       []string
	PermissionKey string
	ScopeBindings []byte
}
