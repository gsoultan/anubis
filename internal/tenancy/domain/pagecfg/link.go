package pagecfg

// Link is one auxiliary link (password reset, support, terms). Bounded in
// number and restricted to http(s): a javascript: or data: href on the login
// page would execute in Anubis's own origin.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

const maxLinks = 5

func (l Link) validate(i int) error {
	field := "links[" + itoa(i) + "]"
	if err := checkText(field+".label", l.Label, 60, true); err != nil {
		return err
	}
	return checkURL(field+".url", l.URL)
}
