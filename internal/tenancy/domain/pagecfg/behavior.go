package pagecfg

// Behavior is sign-out page conduct.
type Behavior struct {
	// Confirm asks before ending the session. Defaults to true: without a
	// confirmation, any page on the internet can sign a user out by pointing
	// an <img> at the logout URL. Annoying rather than dangerous — but it is
	// still an unauthenticated state change, and the confirmation is what
	// makes it deliberate.
	Confirm bool `json:"confirm"`
	// AutoRedirectSeconds bounces to the return URL after signing out.
	// 0 disables; capped so a page cannot pin a user in a redirect loop.
	AutoRedirectSeconds int `json:"auto_redirect_seconds,omitempty"`
	// DefaultReturnURL is where "return to the application" points when the
	// request supplies no post_logout_redirect_uri. Still checked against the
	// application's allowlist at request time — a stored value is not a
	// trusted value.
	DefaultReturnURL string `json:"default_return_url,omitempty"`

	// confirmSet records that the caller supplied `confirm` explicitly, so
	// "absent" can default to true while "false" is honoured.
	confirmSet bool
}

const maxAutoRedirectSeconds = 30

func (b *Behavior) applyDefaults() {
	if !b.confirmSet {
		b.Confirm = true
	}
}

// confirmSet distinguishes "absent" from "explicitly false" for a bool that
// defaults to true. UnmarshalJSON records that the key was present.
func (b *Behavior) UnmarshalJSON(data []byte) error {
	type alias Behavior
	var probe struct {
		alias
		Confirm *bool `json:"confirm"`
	}
	if err := jsonUnmarshal(data, &probe); err != nil {
		return err
	}
	*b = Behavior(probe.alias)
	if probe.Confirm != nil {
		b.Confirm = *probe.Confirm
		b.confirmSet = true
	}
	return nil
}

func (b *Behavior) validate() error {
	if b.AutoRedirectSeconds < 0 || b.AutoRedirectSeconds > maxAutoRedirectSeconds {
		return invalid("behavior.auto_redirect_seconds", "expected 0-30")
	}
	if b.DefaultReturnURL != "" {
		return checkURL("behavior.default_return_url", b.DefaultReturnURL)
	}
	return nil
}
