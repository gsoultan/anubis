package authzdomain

type PermissionRecord struct {
	ID           string
	Key          string
	AppSlug      string
	Resource     string
	Action       string
	Risk         string
	Description  string
	MinAssurance int
	RequiresAMR  []string
	MaxAuthAge   string
	Deprecated   bool
	// Module is the label the app files the permission under (Payables).
	Module string
}
