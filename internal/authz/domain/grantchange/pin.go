package grantchange

// Pin is one row of a grant's scope: a place on one kind of place, picked
// in (or, with Exclude, left out). The JSON shape is what grant_change_items
// stores, so a change recorded today still reads after the Go type moves.
type Pin struct {
	Axis    string `json:"axis"`
	NodeID  string `json:"node"`
	Inherit bool   `json:"inherit"`
	Exclude bool   `json:"exclude,omitempty"`
}
