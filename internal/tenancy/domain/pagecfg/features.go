package pagecfg

// Features toggles what a sign-in page offers. Each is a capability the
// server must also support — showing a self-registration link on a realm
// that forbids it would advertise a door that does not open, so the renderer
// intersects these with realm policy rather than trusting them alone.
type Features struct {
	ShowRealmPicker    bool `json:"show_realm_picker,omitempty"`
	ShowRegistration   bool `json:"show_registration,omitempty"`
	ShowForgotPassword bool `json:"show_forgot_password,omitempty"`
	RememberMe         bool `json:"remember_me,omitempty"`
}
