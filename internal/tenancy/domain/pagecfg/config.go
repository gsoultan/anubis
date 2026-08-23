package pagecfg

// Config is one page's appearance and behaviour. Sign-in pages use Features;
// sign-out pages use Behavior; both share Brand, Layout, Copy and Links.
type Config struct {
	Brand  Brand  `json:"brand"`
	Layout string `json:"layout"`
	Copy   Copy   `json:"copy"`
	Links  []Link `json:"links,omitempty"`

	Features Features `json:"features,omitempty"`
	Behavior Behavior `json:"behavior,omitempty"`
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
