package pagecfg

// Copy is the wording. Fields are plain text: the templates escape them, and
// no field accepts markup — a "footer_html" knob is exactly how a login page
// becomes a phishing surface for its own users.
type Copy struct {
	Heading       string `json:"heading"`
	Subheading    string `json:"subheading,omitempty"`
	UsernameLabel string `json:"username_label"`
	PasswordLabel string `json:"password_label"`
	SubmitLabel   string `json:"submit_label"`
	// Sign-out only. ConfirmHeading and ConfirmBody are shown while ASKING;
	// Heading and Body after the session has ended. One pair cannot do both
	// jobs — "You have been signed out" above a "Sign out" button is nonsense.
	ConfirmHeading string `json:"confirm_heading,omitempty"`
	ConfirmBody    string `json:"confirm_body,omitempty"`
	Body           string `json:"body,omitempty"`
	ReturnLabel    string `json:"return_label,omitempty"`
}

func (c *Copy) applyDefaults(kind Kind) {
	if kind == KindSignout {
		if c.ConfirmHeading == "" {
			c.ConfirmHeading = "Sign out?"
		}
		if c.ConfirmBody == "" {
			c.ConfirmBody = "You will need to sign in again to continue."
		}
		if c.Heading == "" {
			c.Heading = "You have been signed out"
		}
		if c.Body == "" {
			c.Body = "Your session on this device has ended."
		}
		if c.ReturnLabel == "" {
			c.ReturnLabel = "Return to the application"
		}
		return
	}
	if c.Heading == "" {
		c.Heading = "Sign in"
	}
	if c.UsernameLabel == "" {
		c.UsernameLabel = "Username"
	}
	if c.PasswordLabel == "" {
		c.PasswordLabel = "Password"
	}
	if c.SubmitLabel == "" {
		c.SubmitLabel = "Sign in"
	}
}

func (c *Copy) validate() error {
	for field, spec := range map[string]struct {
		value string
		max   int
	}{
		"copy.heading":         {c.Heading, 120},
		"copy.subheading":      {c.Subheading, 240},
		"copy.username_label":  {c.UsernameLabel, 60},
		"copy.password_label":  {c.PasswordLabel, 60},
		"copy.submit_label":    {c.SubmitLabel, 60},
		"copy.confirm_heading": {c.ConfirmHeading, 120},
		"copy.confirm_body":    {c.ConfirmBody, 500},
		"copy.body":            {c.Body, 500},
		"copy.return_label":    {c.ReturnLabel, 60},
	} {
		if err := checkText(field, spec.value, spec.max, false); err != nil {
			return err
		}
	}
	return nil
}
