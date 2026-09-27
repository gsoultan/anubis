package enroll

import (
	"errors"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

func TestOnlyAFreshAnubisSessionMayBindAnAuthenticator(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		aud  []string
		auth time.Time
		want *apperr.Error // nil: allowed
	}{
		{"Anubis's own token, just signed in", []string{"anubis"}, now.Add(-time.Minute), nil},
		{"still inside the window", []string{"anubis"}, now.Add(-bindingWindow), nil},
		// The attack: an application's token, straight after sign-in.
		{"a token issued to an application", []string{"billing"}, now.Add(-time.Minute), apperr.ErrPermissionDenied},
		{"no audience at all", nil, now.Add(-time.Minute), apperr.ErrPermissionDenied},
		{"a sign-in that has gone stale", []string{"anubis"}, now.Add(-bindingWindow - time.Second), apperr.ErrStepUpRequired},
		// A token with no auth_time decodes to the epoch: fail closed.
		{"no sign-in time recorded", []string{"anubis"}, time.Unix(0, 0), apperr.ErrStepUpRequired},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := mayBindAuthenticator(&authctx.Principal{Audience: c.aud, AuthTime: c.auth}, now)
			if c.want == nil {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			// With copies the error, so match the code, not the pointer.
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Code != c.want.Code {
				t.Fatalf("got %v, want %s", err, c.want.Code)
			}
		})
	}
}
