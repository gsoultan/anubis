//go:build integration

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

func pageClient() anubisv1connect.TenantAdminServiceClient {
	return anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
}

// A tenant runs several audiences — staff, partners, customers — and they do
// not share a login screen. This covers the lifecycle of one such page and
// the URL it is published at.
func TestAuthPageLifecycle(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	slug := fmt.Sprintf("probe-%d", time.Now().UnixNano()%1_000_000)

	created, err := pageClient().CreateAuthPage(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateAuthPageRequest{
		Page: &anubisv1.AuthPage{
			Kind: "signin", Slug: slug, Name: "Probe page",
			ConfigJson: `{"brand":{"title":"Probe Ltd","primary_color":"#0f766e"},
			              "layout":"minimal","copy":{"heading":"Probe sign-in"}}`,
		},
	}), token))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	page := created.Msg.Page
	if page.Url == "" || !strings.HasSuffix(page.Url, "/signin/"+slug) {
		t.Fatalf("page URL not reported: %q", page.Url)
	}
	if page.IsDefault {
		t.Fatal("a newly created page must not silently become the default")
	}

	// It renders at its own URL, with its own branding, and nothing else's.
	body := getBody(t, page.Url)
	for _, want := range []string{"Probe Ltd", "Probe sign-in", "#0f766e"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q", want)
		}
	}

	// Updating changes appearance but never the published URL.
	updated, err := pageClient().UpdateAuthPage(ctx, operatorBearer(connect.NewRequest(&anubisv1.UpdateAuthPageRequest{
		Page: &anubisv1.AuthPage{
			Id: page.Id, Name: "Probe page v2",
			ConfigJson: `{"brand":{"title":"Probe Ltd"},"copy":{"heading":"Welcome back"}}`,
		},
	}), token))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Msg.Page.Slug != slug || updated.Msg.Page.Url != page.Url {
		t.Fatalf("update changed the page's identity: %s -> %s", page.Url, updated.Msg.Page.Url)
	}
	if !strings.Contains(getBody(t, page.Url), "Welcome back") {
		t.Fatal("update did not reach the rendered page")
	}

	// Disabling takes it off the internet: a retired design must not survive
	// for anyone who kept the link.
	if _, err := pageClient().UpdateAuthPage(ctx, operatorBearer(connect.NewRequest(&anubisv1.UpdateAuthPageRequest{
		Page: &anubisv1.AuthPage{Id: page.Id, Name: "Probe page v2", Status: "disabled",
			ConfigJson: `{}`},
	}), token)); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if code := getStatus(t, page.Url); code != http.StatusNotFound {
		t.Fatalf("disabled page still served: status %d", code)
	}

	if _, err := pageClient().DeleteAuthPage(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.DeleteAuthPageRequest{Id: page.Id}), token)); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// The default page is load-bearing: /v1/authorize renders it when nothing
// more specific applies, so it must not be deletable and promotion must be
// atomic (exactly one default at a time).
func TestDefaultPageIsProtected(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)

	list, err := pageClient().ListAuthPages(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.ListAuthPagesRequest{Kind: "signout"}), token))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var def *anubisv1.AuthPage
	for _, p := range list.Msg.Pages {
		if p.IsDefault {
			def = p
		}
	}
	if def == nil {
		t.Skip("no default sign-out page in this database")
	}
	if _, err := pageClient().DeleteAuthPage(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.DeleteAuthPageRequest{Id: def.Id}), token)); err == nil {
		t.Fatal("the default page was deletable, which would leave sign-out with nothing to render")
	}

	// Promote a second page and confirm the previous default steps down.
	slug := fmt.Sprintf("alt-%d", time.Now().UnixNano()%1_000_000)
	created, err := pageClient().CreateAuthPage(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateAuthPageRequest{
		Page: &anubisv1.AuthPage{Kind: "signout", Slug: slug, Name: "Alt sign-out", ConfigJson: `{}`},
	}), token))
	if err != nil {
		t.Fatalf("create alt: %v", err)
	}
	if _, err := pageClient().SetDefaultAuthPage(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.SetDefaultAuthPageRequest{Id: created.Msg.Page.Id}), token)); err != nil {
		t.Fatalf("promote: %v", err)
	}
	after, _ := pageClient().ListAuthPages(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.ListAuthPagesRequest{Kind: "signout"}), token))
	defaults := 0
	for _, p := range after.Msg.Pages {
		if p.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("expected exactly one default sign-out page, found %d", defaults)
	}
	// Put the original back so the suite stays re-runnable.
	_, _ = pageClient().SetDefaultAuthPage(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.SetDefaultAuthPageRequest{Id: def.Id}), token))
	_, _ = pageClient().DeleteAuthPage(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.DeleteAuthPageRequest{Id: created.Msg.Page.Id}), token))
}

// The builder stores configuration, not markup. Every one of these would end
// up inside the page Anubis serves on its own origin.
func TestBuilderRefusesHostileConfig(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)

	for name, cfg := range map[string]string{
		"css injection":  `{"brand":{"primary_color":"red;} body{display:none"}}`,
		"javascript url": `{"brand":{"logo_url":"javascript:alert(1)"}}`,
		"unknown layout": `{"layout":"raw-html"}`,
		"script in link": `{"links":[{"label":"x","url":"javascript:fetch('/steal')"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pageClient().CreateAuthPage(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateAuthPageRequest{
				Page: &anubisv1.AuthPage{Kind: "signin", Slug: "hostile-probe",
					Name: "Hostile", ConfigJson: cfg},
			}), token))
			if err == nil {
				t.Fatalf("accepted hostile config: %s", cfg)
			}
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Fatalf("want invalid_argument, got %v", connect.CodeOf(err))
			}
			// Preview must agree with save, or the builder shows a green
			// preview for something that cannot be stored.
			pv, perr := pageClient().PreviewAuthPage(ctx, operatorBearer(connect.NewRequest(
				&anubisv1.PreviewAuthPageRequest{Kind: "signin", ConfigJson: cfg}), token))
			if perr != nil {
				t.Fatalf("preview errored: %v", perr)
			}
			if pv.Msg.Valid {
				t.Fatal("preview called a hostile config valid while save rejected it")
			}
		})
	}
}

// Plain text with angle brackets is legitimate ("Smith & Co <Staff>") and
// must survive to the page ESCAPED, not rejected and not raw.
func TestPageEscapesRatherThanRejectsText(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	slug := fmt.Sprintf("esc-%d", time.Now().UnixNano()%1_000_000)

	created, err := pageClient().CreateAuthPage(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateAuthPageRequest{
		Page: &anubisv1.AuthPage{Kind: "signout", Slug: slug, Name: "Escaping probe",
			// The CONFIRM screen renders confirm_heading; setting only
			// `heading` would leave the probe text off the page entirely.
			ConfigJson: `{"copy":{"confirm_heading":"<script>alert(1)</script> & farewell"}}`},
	}), token))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer pageClient().DeleteAuthPage(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.DeleteAuthPageRequest{Id: created.Msg.Page.Id}), token))

	body := getBody(t, created.Msg.Page.Url)
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatal("page emitted raw script from configuration")
	}
	if !strings.Contains(body, "&lt;script&gt;") && !strings.Contains(body, "&amp;") {
		t.Fatal("page did not render the escaped text at all")
	}
	// And the page must forbid framing and scripts outright.
	resp, err := http.Get(created.Msg.Page.Url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("login/logout page is frameable: clickjacking risk")
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("missing restrictive CSP: %q", csp)
	}
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func getStatus(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}
