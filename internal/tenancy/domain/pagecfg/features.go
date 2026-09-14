package pagecfg

// Features toggles what a sign-in page offers. Each is a capability the
// server must also support — showing a self-registration link on a realm
// that forbids it would advertise a door that does not open, so the renderer
// intersects these with realm policy rather than trusting them alone.
//
// There is no show_forgot_password. There was, and it rendered NOWHERE: no
// template drew it, no handler answered it, and the console offered a switch
// that did nothing while the builder's preview drew a link users would never
// see. A toggle that cannot change the page is worse than a missing feature,
// because somebody trusts it and stops looking.
//
// Anubis has no password-reset flow to link to. A tenant that runs its own
// puts it in Links, which is a label and a URL and renders exactly where the
// operator expects. Stored configs carrying the old key still parse — Parse
// ignores unknown fields on purpose — they simply stop pretending.
type Features struct {
	ShowRealmPicker  bool `json:"show_realm_picker,omitempty"`
	ShowRegistration bool `json:"show_registration,omitempty"`
	RememberMe       bool `json:"remember_me,omitempty"`
}
