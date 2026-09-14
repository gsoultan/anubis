package pagecfg

// Config is one page's appearance and behaviour. Sign-in pages use Features;
// sign-out pages use Behavior; both share Brand, Layout, Copy and Links.
type Config struct {
	Brand  Brand  `json:"brand"`
	Layout string `json:"layout"`
	Copy   Copy   `json:"copy"`
	Links  []Link `json:"links,omitempty"`
	// Sections is the order of the blocks, and by omission which of them
	// render at all. Absent means the order this page has always used.
	Sections []string `json:"sections,omitempty"`

	Features Features `json:"features,omitempty"`
	Behavior Behavior `json:"behavior,omitempty"`
	Motion   Motion   `json:"motion,omitempty"`
}

// Layout choices. Adding one means adding a template, which is why this is a
// closed set rather than a free string.
const (
	LayoutCentered = "centered"
	LayoutSplit    = "split"
	LayoutMinimal  = "minimal"
)

var layouts = map[string]bool{LayoutCentered: true, LayoutSplit: true, LayoutMinimal: true}

func (c *Config) applyDefaults(kind Kind) {
	c.Brand.applyDefaults()
	if c.Layout == "" {
		c.Layout = LayoutCentered
	}
	c.Copy.applyDefaults(kind)
	c.applySectionDefaults()
	c.Motion.applyDefaults()
	if kind == KindSignout {
		c.Behavior.applyDefaults()
	}
}

// Validate reports the FIRST problem with a machine-readable field name, so
// the console can point at the input that is wrong instead of showing "invalid
// configuration".
func (c *Config) Validate(kind Kind) error {
	if !layouts[c.Layout] {
		return invalid("layout", c.Layout)
	}
	if err := c.Brand.validate(); err != nil {
		return err
	}
	if err := c.Copy.validate(); err != nil {
		return err
	}
	if err := c.validateSections(); err != nil {
		return err
	}
	if err := c.Motion.validate(); err != nil {
		return err
	}
	if len(c.Links) > maxLinks {
		return invalid("links", "at most 5 links")
	}
	for i, l := range c.Links {
		if err := l.validate(i); err != nil {
			return err
		}
	}
	if kind == KindSignout {
		return c.Behavior.validate()
	}
	return nil
}

// Sections is the ORDER of the blocks on the page, and by omission which
// blocks are on it at all.
//
// It exists because the builder needed something to drag. Everything else in
// this file's neighbours is a value — a colour, a label, a switch — and a
// visual builder over nothing but values is a form with a picture next to it.
// Order is the one structural choice this page has that is still expressible
// as tokens, which is the line pagecfg does not cross (see pagecfg.go: no
// markup, ever, on the one screen where people type their password).
//
// It is a LIST OF NAMES, not a tree and not a layout. A section cannot be
// nested, styled, repeated or invented; the renderer knows five blocks and
// will draw each at most once. What an operator can say is "links above the
// form" or "no logo on this one" — and nothing whose worst case is a script
// tag.
const (
	SectionLogo       = "logo"
	SectionHeading    = "heading"
	SectionSubheading = "subheading"
	SectionForm       = "form"
	SectionLinks      = "links"
)

// DefaultSections is the order the page has always rendered in, so a config
// written before this existed renders identically after it.
func DefaultSections() []string {
	return []string{SectionLogo, SectionHeading, SectionSubheading, SectionForm, SectionLinks}
}

var sections = map[string]bool{
	SectionLogo: true, SectionHeading: true, SectionSubheading: true,
	SectionForm: true, SectionLinks: true,
}

func (c *Config) applySectionDefaults() {
	if len(c.Sections) == 0 {
		c.Sections = DefaultSections()
	}
}

func (c *Config) validateSections() error {
	seen := make(map[string]bool, len(c.Sections))
	form := false
	for _, s := range c.Sections {
		if !sections[s] {
			return invalid("sections", s)
		}
		if seen[s] {
			// Drawing a block twice is not a layout an operator meant; it is a
			// drag that landed wrong, and it would double the sign-in form.
			return invalid("sections", "duplicate section "+s)
		}
		seen[s] = true
		if s == SectionForm {
			form = true
		}
	}
	if !form {
		// The form is the page. A sign-in screen without one is a door with no
		// handle, and it would be reachable from a single stray drag.
		return invalid("sections", "the form cannot be removed")
	}
	return nil
}
