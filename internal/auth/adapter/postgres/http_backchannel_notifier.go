package authpg

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPBackchannelNotifier delivers logout tokens as
// application/x-www-form-urlencoded POSTs (OIDC back-channel logout shape).
// Fire-and-forget with a hard timeout; failures are logged loudly because a
// missed back-channel logout leaves an application session alive.
type HTTPBackchannelNotifier struct {
	client *http.Client
	logger *slog.Logger
}

func NewHTTPBackchannelNotifier(logger *slog.Logger) *HTTPBackchannelNotifier {
	return &HTTPBackchannelNotifier{
		client: &http.Client{Timeout: 5 * time.Second},
		logger: logger,
	}
}

func (n *HTTPBackchannelNotifier) NotifyLogout(ctx context.Context, uri string, logoutToken string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		form := url.Values{"logout_token": {logoutToken}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri,
			strings.NewReader(form.Encode()))
		if err != nil {
			n.logger.Error("backchannel logout: bad uri", "uri", uri, "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := n.client.Do(req)
		if err != nil {
			n.logger.Error("backchannel logout delivery failed", "uri", uri, "error", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			n.logger.Error("backchannel logout rejected", "uri", uri, "status", resp.StatusCode)
		}
	}()
}
