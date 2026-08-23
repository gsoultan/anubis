package pagecfg

// Brand is the visual identity. Every value lands in CSS or an <img src>, so
// every value is validated: a colour that is not a colour is a stylesheet
// injection, and a logo URL that is not http(s) is a script vector.
type Brand struct {
	Title           string `json:"title"`
	LogoURL         string `json:"logo_url,omitempty"`
	PrimaryColor    string `json:"primary_color"`
	BackgroundColor string `json:"background_color"`
	TextColor       string `json:"text_color"`
	CornerRadius    string `json:"corner_radius"`
	Font            string `json:"font"`
}

const (
	RadiusNone = "none"
	RadiusSm   = "sm"
	RadiusMd   = "md"
	RadiusLg   = "lg"
	RadiusFull = "full"

	FontSystem = "system"
	FontSerif  = "serif"
	FontMono   = "mono"
)

var (
	radii = map[string]bool{RadiusNone: true, RadiusSm: true, RadiusMd: true,
		RadiusLg: true, RadiusFull: true}
	fonts = map[string]bool{FontSystem: true, FontSerif: true, FontMono: true}
)

func (b *Brand) applyDefaults() {
	if b.Title == "" {
		b.Title = "Anubis"
	}
	if b.PrimaryColor == "" {
		b.PrimaryColor = "#4f46e5"
	}
	if b.BackgroundColor == "" {
		b.BackgroundColor = "#f6f6f7"
	}
	if b.TextColor == "" {
		b.TextColor = "#111827"
	}
	if b.CornerRadius == "" {
		b.CornerRadius = RadiusMd
	}
	if b.Font == "" {
		b.Font = FontSystem
	}
}

func (b *Brand) validate() error {
	if err := checkText("brand.title", b.Title, 60, true); err != nil {
		return err
	}
	for field, value := range map[string]string{
		"brand.primary_color":    b.PrimaryColor,
		"brand.background_color": b.BackgroundColor,
		"brand.text_color":       b.TextColor,
	} {
		if !validColor(value) {
			return invalid(field, "expected #rgb or #rrggbb")
		}
	}
	if !radii[b.CornerRadius] {
		return invalid("brand.corner_radius", b.CornerRadius)
	}
	if !fonts[b.Font] {
		return invalid("brand.font", b.Font)
	}
	if b.LogoURL != "" {
		if err := checkURL("brand.logo_url", b.LogoURL); err != nil {
			return err
		}
	}
	return nil
}

// RadiusCSS maps the token to a value the template can emit safely.
func (b Brand) RadiusCSS() string {
	switch b.CornerRadius {
	case RadiusNone:
		return "0"
	case RadiusSm:
		return "4px"
	case RadiusLg:
		return "20px"
	case RadiusFull:
		return "9999px"
	default:
		return "12px"
	}
}

// FontCSS maps the token to a font stack; the admin never supplies one.
func (b Brand) FontCSS() string {
	switch b.Font {
	case FontSerif:
		return "Georgia, 'Times New Roman', serif"
	case FontMono:
		return "ui-monospace, SFMono-Regular, Menlo, monospace"
	default:
		return "system-ui, -apple-system, 'Segoe UI', sans-serif"
	}
}
