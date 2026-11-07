package tenancycache

import (
	"time"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// slugEntry is one remembered tenant and the moment it must be read again.
type slugEntry struct {
	ref     tenancydomain.TenantRef
	expires time.Time
}
