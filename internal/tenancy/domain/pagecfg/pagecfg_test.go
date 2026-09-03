package pagecfg

import (
	"strings"
	"testing"
)

func TestEmptyConfigRendersACompletePage(t *testing.T) {
	// A tenant that configures nothing must still get a working page: the
	// builder's defaults are what stands between "no config yet" and a blank
	// login screen.
	for _, kind := range []Kind{KindSignin, KindSignout} {
		c, err := Parse(kind, nil)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if c.Brand.Title == "" || c.Copy.Heading == "" || c.Layout == "" {
			t.Fatalf("%s: defaults incomplete: %+v", kind, c)
		}
		if !validColor(c.Brand.PrimaryColor) {
			t.Fatalf("%s: default colour invalid", kind)
		}
	}
	// Sign-out defaults to asking first: an unauthenticated GET that ends a
	// session is something any page on the internet can trigger.
	c, _ := Parse(KindSignout, nil)
	if !c.Behavior.Confirm {
		t.Fatal("sign-out must default to confirming")
	}
	// …but an explicit false is honoured.
	c, err := Parse(KindSignout, []byte(`{"behavior":{"confirm":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Behavior.Confirm {
		t.Fatal("explicit confirm:false was ignored")
	}
}

// The page is rendered on Anubis's own origin, on the screen where users type
// their password. Every one of these is a real attempt to escape the token
// set into markup, CSS or script.
func TestHostileConfigsRejected(t *testing.T) {
	cases := []struct {
		name, json string
	}{
		{"css injection via colour",
			`{"brand":{"primary_color":"red;} body{display:none} .x{color:red"}}`},
		{"css keyword colour",
			`{"brand":{"primary_color":"red"}}`},
		{"css var() colour",
			`{"brand":{"primary_color":"var(--anything)"}}`},
		{"url() in colour",
			`{"brand":{"primary_color":"url(https://evil.example/x)"}}`},
		{"javascript: logo",
			`{"brand":{"logo_url":"javascript:alert(1)"}}`},
		{"data: logo",
			`{"brand":{"logo_url":"data:text/html;base64,PHNjcmlwdD4="}}`},
		{"javascript: link",
			`{"links":[{"label":"Help","url":"javascript:alert(1)"}]}`},
		{"unknown layout",
			`{"layout":"custom-html"}`},
		{"unknown font",
			`{"brand":{"font":"'; background:url(evil)"}}`},
		{"unknown radius",
			`{"brand":{"corner_radius":"9999px; position:fixed"}}`},
		{"control characters in copy",
			`{"copy":{"heading":"Sign in\u0000<script>"}}`},
		{"too many links",
			`{"links":[{"label":"a","url":"https://a.example"},{"label":"b","url":"https://b.example"},{"label":"c","url":"https://c.example"},{"label":"d","url":"https://d.example"},{"label":"e","url":"https://e.example"},{"label":"f","url":"https://f.example"}]}`},
		{"absurd auto redirect",
			`{"behavior":{"auto_redirect_seconds":86400}}`},
		{"malformed json",
			`{"brand":`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(KindSignout, []byte(c.json)); err == nil {
				t.Fatalf("accepted hostile config: %s", c.json)
			}
		})
	}
}

// Plain text is allowed to contain angle brackets — the templates escape it.
// Rejecting it would make the builder useless for names like "Smith & Co".
func TestOrdinaryTextIsAccepted(t *testing.T) {
	c, err := Parse(KindSignin, []byte(`{
		"brand":{"title":"Smith & Co <Staff>","primary_color":"#0A7","logo_url":"https://cdn.example/logo.png"},
		"layout":"split",
		"copy":{"heading":"Welcome back","username_label":"Work email"},
		"links":[{"label":"Forgot password?","url":"https://help.example/reset"}],
		"features":{"show_forgot_password":true}
	}`))
	if err != nil {
		t.Fatalf("rejected a legitimate config: %v", err)
	}
	if c.Brand.Title != "Smith & Co <Staff>" || c.Layout != LayoutSplit {
		t.Fatalf("config mangled: %+v", c.Brand)
	}
	if c.Brand.RadiusCSS() == "" || !strings.Contains(c.Brand.FontCSS(), "system-ui") {
		t.Fatal("token mapping produced no CSS")
	}
}

// Unknown keys must survive a round trip through an older build rather than
// failing it: the console ships independently of the server.
func TestUnknownFieldsAreTolerated(t *testing.T) {
	if _, err := Parse(KindSignin, []byte(`{"brand":{"title":"X"},"future_knob":{"a":1}}`)); err != nil {
		t.Fatalf("unknown field rejected: %v", err)
	}
}

func TestErrorNamesTheField(t *testing.T) {
	// The console needs to point at the offending input, so the error has to
	// carry which field failed.
	_, err := Parse(KindSignin, []byte(`{"brand":{"primary_color":"nope"}}`))
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "primary_color") && !strings.Contains(fieldOf(err), "primary_color") {
		t.Fatalf("error does not name the field: %v", err)
	}
}

func fieldOf(err error) string {
	type detailed interface{ Error() string }
	if d, ok := err.(detailed); ok {
		return d.Error()
	}
	return ""
}

func TestRoundTrip(t *testing.T) {
	in := []byte(`{"brand":{"title":"Portal","primary_color":"#123456"},"layout":"minimal"}`)
	c, err := Parse(KindSignin, in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(KindSignin, raw)
	if err != nil {
		t.Fatal(err)
	}
	if again.Brand.Title != "Portal" || again.Brand.PrimaryColor != "#123456" || again.Layout != LayoutMinimal {
		t.Fatalf("round trip lost data: %+v", again)
	}
}

// Motion is a closed set for the same reason Layout is: the config is a token
// set the server renders, never CSS it is handed. If this ever accepts a free
// string, a tenant can express a two-second bounce on a credential page — or
// worse, something that is not an animation at all.
func TestOnlyKnownEntrancesAreAccepted(t *testing.T) {
	for _, ok := range []string{"none", "fade", "rise"} {
		c := &Config{Motion: Motion{Entrance: ok}}
		c.applyDefaults(KindSignin)
		if err := c.Motion.validate(); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"bounce", "spin", "FADE", "fade;",
		"1s infinite", "none:hover", "</style><script>",
	} {
		if err := (&Motion{Entrance: bad}).validate(); err == nil {
			t.Errorf("%q was accepted as an entrance", bad)
		}
	}
}

// The default must be none, so no existing installation starts animating
// because it upgraded.
func TestMotionDefaultsToNone(t *testing.T) {
	c := &Config{}
	c.applyDefaults(KindSignin)
	if c.Motion.Entrance != EntranceNone {
		t.Errorf("default entrance = %q, want none", c.Motion.Entrance)
	}
	if c.Motion.Animated() {
		t.Error("the default config reports itself as animated")
	}
}

// The error names the field, so the console can point at the input rather
// than saying "invalid configuration".
func TestABadEntranceNamesItsField(t *testing.T) {
	err := (&Motion{Entrance: "bounce"}).validate()
	if err == nil || !strings.Contains(err.Error(), "motion.entrance") {
		t.Fatalf("error does not name the field: %v", err)
	}
}
