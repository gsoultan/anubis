package grantchange

// Item is one grant a change alters, with its state either side.
type Item struct {
	GrantID    string `json:"grant_id"`
	IdentityID string `json:"-"`
	Username   string `json:"-"`
	Before     State  `json:"before"`
	After      State  `json:"after"`
}
