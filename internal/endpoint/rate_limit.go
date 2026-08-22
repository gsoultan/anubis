package endpoint

import (
	"context"
	"strconv"

	"github.com/go-kit/kit/endpoint"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/ratelimit"
)

// KeyFunc extracts the rate-limit axes from a request. Three axes (ip,
// account, tenant): per-account is the one that stops credential stuffing.
type KeyFunc func(ctx context.Context, req any) []ratelimit.KeyLimit

// RateLimit denies before any work happens — attack traffic must never reach
// the KDF or the database.
func RateLimit(limiter *ratelimit.Limiter, keys KeyFunc) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			if ok, retry := limiter.AllowAll(keys(ctx, req)...); !ok {
				secs := int(retry.Seconds()) + 1
				return nil, domain.ErrRateLimited.With("retry_after", strconv.Itoa(secs))
			}
			return next(ctx, req)
		}
	}
}

// LoginKeys is the standard 3-axis extractor for credential-bearing calls.
func LoginKeys(ipPerMin, acctPerMin, tenantPerMin float64) KeyFunc {
	return func(ctx context.Context, req any) []ratelimit.KeyLimit {
		type tenanted interface{ RateTenant() string }
		type accounted interface{ RateAccount() string }
		var keys []ratelimit.KeyLimit
		if ip := authctx.ClientIP(ctx); ip != "" {
			keys = append(keys, ratelimit.KeyLimit{
				Key:   "ip:" + ip,
				Limit: ratelimit.Limit{PerMinute: ipPerMin, Burst: ipPerMin},
			})
		}
		if t, ok := req.(tenanted); ok && t.RateTenant() != "" {
			keys = append(keys, ratelimit.KeyLimit{
				Key:   "tenant:" + t.RateTenant(),
				Limit: ratelimit.Limit{PerMinute: tenantPerMin, Burst: tenantPerMin},
			})
		}
		if a, ok := req.(accounted); ok && a.RateAccount() != "" {
			keys = append(keys, ratelimit.KeyLimit{
				Key:   "acct:" + a.RateAccount(),
				Limit: ratelimit.Limit{PerMinute: acctPerMin, Burst: acctPerMin},
			})
		}
		return keys
	}
}
