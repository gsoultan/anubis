package apiconnect

import (
	"testing"

	controlapp "github.com/gsoultan/anubis/internal/control/app"
)

// The interceptor cannot import a bounded context, so it repeats the
// audience string. If the two ever drift, every operator token silently
// stops being recognised and the console just says "permission denied".
func TestPlatformAudienceMatchesTheControlContext(t *testing.T) {
	if platformAudience != controlapp.PlatformAudience {
		t.Fatalf("interceptor has %q, control mints %q", platformAudience, controlapp.PlatformAudience)
	}
}
