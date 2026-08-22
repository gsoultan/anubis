package authzapp

type AuthorizeInput struct {
	Subject    string
	Permission string
	Scopes     map[string]string
	AMR        []string
	AuthTime   int64
}
