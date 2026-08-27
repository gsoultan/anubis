package feed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// HTTPFetcher pulls a JSON array of feed rows from config.url:
//
//	{"url": "https://erp.internal/api/cost-centers",
//	 "auth_header": "Bearer <token>",           // optional, sent as Authorization
//	 "default_node_type": "cost_center"}
type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{client: &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
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

func (f *HTTPFetcher) Fetch(ctx context.Context, source scopedomain.SyncSourceRecord) ([]scopedomain.SyncFeedRow, error) {
	var cfg httpConfig
	if err := json.Unmarshal(source.Config, &cfg); err != nil || cfg.URL == "" {
		return nil, apperr.ErrInvalidArgument.With("config", "http source needs url")
	}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return nil, apperr.ErrInvalidArgument.With("url", "http(s) only")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.Wrap(err)
	}
	// Same egress policy as the database kinds: a URL is a request to connect
	// somewhere, and a feed has no business at a metadata endpoint.
	if err := allowExternalHost(req.URL.Hostname()); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if cfg.AuthHeader != "" {
		req.Header.Set("Authorization", cfg.AuthHeader)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, apperr.ErrUnavailableFeed.Wrap(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperr.ErrUnavailableFeed.With("status", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, apperr.ErrUnavailableFeed.Wrap(err)
	}
	if len(raw) > maxBodyBytes {
		return nil, apperr.ErrInvalidArgument.With("body", "feed exceeds 10MiB")
	}
	var rows []scopedomain.SyncFeedRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, apperr.ErrInvalidArgument.With("body", "expected a JSON array of {ref,parent_ref,name,node_type}").Wrap(err)
	}
	if len(rows) > maxRows {
		return nil, tooMany(len(rows))
	}
	return rows, nil
}
