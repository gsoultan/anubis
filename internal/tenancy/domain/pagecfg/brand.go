package pagecfg

import (
	"strings"

	"github.com/gsoultan/anubis/internal/tenancy/domain/pagecfg/palette"
)

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
//
// NO QUOTED FAMILY NAMES. html/template refuses a quote inside a CSS value —
// it is how a value escapes its declaration — and replaces the whole thing
// with the string ZgotmplZ. The page then rendered `font-family:ZgotmplZ`,
// which is not a font, so every hosted page fell back to the browser default
// and two of the three typeface choices did nothing at all. It failed in
// silence: the config was valid, the template was valid, and the preview drew
// its own font stack so the builder looked right.
//
// Multi-word families are legal unquoted (CSS Fonts 3 §3.1: a sequence of
// identifiers), so the fix costs nothing. Anything added here must stay
// quote-free, and page_template_test.go fails the build if it does not.
func (b Brand) FontCSS() string {
	switch b.Font {
	case FontSerif:
		return "Georgia, Times New Roman, serif"
	case FontMono:
		return "ui-monospace, SFMono-Regular, Menlo, monospace"
	default:
		return "system-ui, -apple-system, Segoe UI, sans-serif"
	}
}

// Initial is the one-letter mark shown when no logo URL is set. The console's
// preview has always drawn it; the template drew nothing, so a tenant with no
// logo got a blank space where the builder promised a badge.
func (b Brand) Initial() string {
	for _, r := range b.Title {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// --- derived colours ---------------------------------------------------------
//
// The template used to hardcode a white card and white button text. Both are
// wrong for half the brands the existing token set can express: a dark
// background left white text on a white card, and a light brand colour left
// white text on a yellow button. Neither is exotic — they are the first two
// things anybody tries after pasting in their own palette.
//
// Deriving beats adding tokens. Another colour input is another thing to get
// wrong, and the answer here is a function of the palette rather than an
// opinion about it. What is NOT derived is text-on-card contrast: that one is
// the operator's own choice, and the builder warns instead of overruling.

// DarkTheme is the page's own answer to "is this a dark page", which decides
// the card and every line on it.
func (b Brand) DarkTheme() bool { return palette.Dark(b.BackgroundColor) }

// SurfaceCSS is the card. White on a light page, exactly as before; a lift out
// of the background on a dark one, because a white card on a dark page with
// the light text that page needs is an invisible form.
func (b Brand) SurfaceCSS() string {
	if b.DarkTheme() {
		return palette.Mix(b.BackgroundColor, "#ffffff", 0.09)
	}
	return "#ffffff"
}

// OnPrimaryCSS is the label on a brand-coloured button. A brand is a brand:
// some of them are yellow, and white-on-yellow is a button whose words nobody
// can read.
func (b Brand) OnPrimaryCSS() string {
	if palette.Dark(b.PrimaryColor) {
		return "#ffffff"
	}
	return "#111111"
}

// MutedCSS is secondary text — the subheading, a hint under a field. The page's
// own text colour faded toward its own card, so it stays legible on a white
// card and a dark one alike. The template used to say #555, which is invisible
// on anything dark.
func (b Brand) MutedCSS() string { return palette.Mix(b.TextColor, b.SurfaceCSS(), 0.38) }

// BorderCSS is every hairline: input outlines, dividers.
func (b Brand) BorderCSS() string { return palette.Mix(b.TextColor, b.SurfaceCSS(), 0.78) }

// FieldCSS is the inside of an input. On a dark page a white input is a slab of
// glare; on a light one it stays white.
func (b Brand) FieldCSS() string {
	if b.DarkTheme() {
		return palette.Mix(b.BackgroundColor, "#ffffff", 0.14)
	}
	return "#ffffff"
}
