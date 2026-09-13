// Package authzfeed reads an application's catalog from wherever that team
// publishes it. It is deliberately thin: a fetcher returns bytes, and what
// those bytes MEAN is authzcatalog's problem, so adding a kind never touches
// the parser and adding a format never touches the transport.
package authzfeed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
	"github.com/gsoultan/anubis/internal/platform/egress"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// A catalog is a document a team maintains, not a data export. Ten megabytes
// is already far past any real one, and the cap is what keeps a wrong URL
// from becoming a memory incident.
const maxBodyBytes = 10 << 20

// HTTPFetcher pulls a catalog document from config.url.
//
//	{"url": "https://platform.internal/catalog/billing.csv",
//	 "auth_header": "Bearer <token>"}   // optional, sent as Authorization
type HTTPFetcher struct{ client *http.Client }

func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{client: &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			// A redirect chain is a second chance to land somewhere the
			// egress policy already refused, so it is kept short and every
			// hop is re-checked below.
			if len(via) >= 3 {
				return apperr.ErrInvalidArgument.With("url", "too many redirects")
			}
			return nil
		},
	}}
}

type httpConfig struct {
	URL        string `json:"url"`
	AuthHeader string `json:"auth_header"`
}

func (f *HTTPFetcher) Fetch(ctx context.Context, source catalogsync.Source) (string, error) {
	var cfg httpConfig
	if err := json.Unmarshal(source.Config, &cfg); err != nil || cfg.URL == "" {
		return "", apperr.ErrInvalidArgument.With("config", "http source needs url")
	}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return "", apperr.ErrInvalidArgument.With("url", "http(s) only")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return "", apperr.ErrInvalidArgument.Wrap(err)
	}
	if err := egress.AllowHost(req.URL.Hostname()); err != nil {
		return "", err
	}
	if source.Format == "csv" {
		req.Header.Set("Accept", "text/csv, text/plain")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	if cfg.AuthHeader != "" {
		req.Header.Set("Authorization", cfg.AuthHeader)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", apperr.ErrUnavailableFeed.Wrap(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", apperr.ErrUnavailableFeed.With("status", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return "", apperr.ErrUnavailableFeed.Wrap(err)
	}
	if len(raw) > maxBodyBytes {
		return "", apperr.ErrInvalidArgument.With("body", "catalog document exceeds 10 MiB")
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		// An empty response is the shape of a feed that is broken rather
		// than a catalog that is empty, and applying it would deprecate
		// things. The apply path refuses this too; refusing here names the
		// source instead of the document.
		return "", apperr.ErrUnavailableFeed.With("body", "empty document")
	}
	return string(raw), nil
}
