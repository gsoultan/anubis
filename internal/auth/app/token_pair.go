package authapp

// TokenPair is what a successful authentication returns. RefreshToken may be
// empty for flows that re-issue access without rotating refresh (scope
// switch).
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int
	SessionID    string
}
