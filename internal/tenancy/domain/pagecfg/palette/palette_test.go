package palette

import "testing"

func TestDarkCrossover(t *testing.T) {
	cases := []struct {
		hex  string
		dark bool
	}{
		{"#000000", true}, {"#ffffff", false}, {"#fff", false},
		{"#0b0b0f", true}, {"#f6f6f7", false},
		// Mid grey is the one people guess wrong: white text on it is poor.
		{"#808080", false},
		// A saturated brand yellow is LIGHT however vivid it looks.
		{"#ffd400", false},
		{"#4f46e5", true},
		// Unparseable reports light, so a typo never flips a page to dark.
		{"not-a-colour", false},
	}
	for _, c := range cases {
		if got := Dark(c.hex); got != c.dark {
			t.Errorf("Dark(%s) = %v, want %v (luminance %.3f)", c.hex, got, c.dark, Luminance(c.hex))
		}
	}
}

func TestMix(t *testing.T) {
	if got := Mix("#000000", "#ffffff", 0.5); got != "#7f7f7f" {
		t.Errorf("midpoint = %s", got)
	}
	if got := Mix("#000000", "#ffffff", 0); got != "#000000" {
		t.Errorf("t=0 = %s", got)
	}
	if got := Mix("#abc", "#abc", 1); got != "#aabbcc" {
		t.Errorf("shorthand expansion = %s", got)
	}
	if got := Mix("bad", "#fff", 0.5); got != "bad" {
		t.Errorf("unparseable should pass through, got %s", got)
	}
}

func TestContrast(t *testing.T) {
	if got := Contrast("#000", "#fff"); got < 20.9 || got > 21.1 {
		t.Errorf("black on white = %.2f, want 21", got)
	}
	// White on this brand yellow is the failure the template used to ship.
	if got := Contrast("#ffffff", "#ffd400"); got >= 4.5 {
		t.Errorf("white on yellow = %.2f, expected it to fail the body-text bar", got)
	}
}
