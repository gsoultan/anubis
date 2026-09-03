package pagecfg

// Motion is deliberately one field with three values.
//
// A sign-in page's job is to be fast and get out of the way. It is the page a
// person sees when they are trying to do something else, often on a bad
// connection, sometimes twice because the first attempt failed — and it is the
// one page where a delay reads as "did that work?". Everything a brand might
// want from motion here is served by the card arriving calmly; everything past
// that (drifting backgrounds, staged reveals, anything looping) buys nothing
// and costs first paint on the page least able to afford it.
//
// It is also an enum rather than a duration-and-easing pair, for the same
// reason Layout is a closed set: the config is a token set that the server
// renders, never markup or CSS it is handed. A tenant cannot express a
// two-second bounce because there is no way to write one down.
//
// Whatever is chosen, the template emits it only inside
// `@media (prefers-reduced-motion: no-preference)`. Someone who has asked
// their system for less motion gets none, and so does anyone whose browser
// does not answer the question — the animation is opt-in at the media query,
// not opt-out.
type Motion struct {
	Entrance string `json:"entrance,omitempty"`
}

const (
	// EntranceNone is the default: existing installations animate nothing
	// until somebody chooses otherwise.
	EntranceNone = "none"
	// EntranceFade is opacity only — the cheapest thing that still reads as
	// deliberate.
	EntranceFade = "fade"
	// EntranceRise is opacity plus a small upward translate. Both properties
	// are compositor-friendly, so it does not cause layout.
	EntranceRise = "rise"
)

var entrances = map[string]bool{
	EntranceNone: true, EntranceFade: true, EntranceRise: true,
}

func (m *Motion) applyDefaults() {
	if m.Entrance == "" {
		m.Entrance = EntranceNone
	}
}

func (m *Motion) validate() error {
	if !entrances[m.Entrance] {
		return invalid("motion.entrance", m.Entrance)
	}
	return nil
}

// Animated reports whether the template needs to emit any keyframes at all.
// Kept on the type so the template asks a question rather than comparing
// strings, which is what would rot when a value is added.
func (m Motion) Animated() bool { return m.Entrance != EntranceNone && m.Entrance != "" }
