package authhttp

/* The hosted page is the one screen between a person and the thing they came
   to use, and until now nothing rendered it in a test — the template could
   only be wrong in production. These render it for real and assert the three
   things that break silently: the order blocks appear in, the mobile rules
   that stop iOS zooming a form out from under somebody's thumb, and the
   colours derived for a palette nobody at Anubis chose.

   `type="hidden"` is why the omitted-section check looks for a nonsense word
   rather than the obvious one: the login form is full of hidden inputs. */

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/anubis/internal/tenancy/domain/pagecfg"
)

func render(t *testing.T, kind pagecfg.Kind, raw string, mutate func(*PageView)) string {
	t.Helper()
	cfg, err := pagecfg.Parse(kind, []byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	v := PageView{Cfg: cfg, Kind: string(kind), Tenant: "acme", Realm: "internal"}
	if mutate != nil {
		mutate(&v)
	}
	rec := httptest.NewRecorder()
	NewPageRenderer().Render(rec, 200, v)
	return rec.Body.String()
}

func TestDefaultSectionOrder(t *testing.T) {
	html := render(t, pagecfg.KindSignin, `{"brand":{"title":"Impack"},"copy":{"subheading":"Sub"},"links":[{"label":"Help","url":"https://x.example"}]}`, nil)
	for _, want := range []string{`class="mark"`, "Impack", "<h1>", "Sub", `name="password"`, "Help", "font:inherit", "100dvh", `autocapitalize="none"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(html, "{{") {
		t.Error("unrendered template action")
	}
	order := []string{`class="mark"`, "<h1>", `class="sub"`, `name="password"`, "Help"}
	at := 0
	for _, s := range order {
		i := strings.Index(html[at:], s)
		if i < 0 {
			t.Fatalf("out of order or missing: %s", s)
		}
		at += i
	}
}

func TestReorderedSections(t *testing.T) {
	html := render(t, pagecfg.KindSignin,
		`{"sections":["heading","form","logo"],"brand":{"title":"Zed"},"copy":{"subheading":"SUBTEXT"}}`, nil)
	if strings.Contains(html, "SUBTEXT") {
		t.Error("a section left out of the list still rendered")
	}
	if strings.Index(html, "<h1>") > strings.Index(html, `name="password"`) {
		t.Error("heading did not come first")
	}
	if strings.Index(html, `class="mark"`) < strings.Index(html, `name="password"`) {
		t.Error("logo did not come last")
	}
}

func TestDarkThemeAndSignout(t *testing.T) {
	html := render(t, pagecfg.KindSignin, `{"brand":{"background_color":"#0b0b0f","text_color":"#f5f5f5","primary_color":"#ffd400"}}`, nil)
	if !strings.Contains(html, "--surface:#202024") || !strings.Contains(html, "--on-brand:#111111") {
		t.Errorf("dark theme not derived:\n%s", html[:strings.Index(html, "*{box-sizing")])
	}
	out := render(t, pagecfg.KindSignout, `{}`, func(v *PageView) { v.LogoutCSRF = "tok" })
	if !strings.Contains(out, `action="/v1/logout"`) || !strings.Contains(out, "tok") {
		t.Error("sign-out form missing")
	}
}

func TestSectionsRefusals(t *testing.T) {
	if _, err := pagecfg.Parse(pagecfg.KindSignin, []byte(`{"sections":["heading"]}`)); err == nil {
		t.Fatal("a page with no form was accepted")
	}
	if _, err := pagecfg.Parse(pagecfg.KindSignin, []byte(`{"sections":["form","form"]}`)); err == nil {
		t.Fatal("a duplicated section was accepted")
	}
	if _, err := pagecfg.Parse(pagecfg.KindSignin, []byte(`{"sections":["form","<script>"]}`)); err == nil {
		t.Fatal("an unknown section was accepted")
	}
}

// html/template replaces a CSS value it will not trust with the literal
// ZgotmplZ, and a page whose font-family is ZgotmplZ renders in whatever the
// browser felt like. That shipped for as long as the font tokens have existed,
// because nothing rendered the template in a test and the console preview drew
// its own stack. Every token, every time.
func TestFontStacksSurviveEscaping(t *testing.T) {
	for _, font := range []string{"system", "serif", "mono"} {
		html := render(t, pagecfg.KindSignin, `{"brand":{"font":"`+font+`"}}`, nil)
		if strings.Contains(html, "ZgotmplZ") {
			t.Errorf("%s: template refused a value and emitted ZgotmplZ", font)
		}
		if !strings.Contains(html, "--font:") {
			t.Errorf("%s: no font declaration at all", font)
		}
	}
	if html := render(t, pagecfg.KindSignin, `{"brand":{"font":"serif"}}`, nil); !strings.Contains(html, "Georgia") {
		t.Error("serif did not reach the stylesheet")
	}
}
