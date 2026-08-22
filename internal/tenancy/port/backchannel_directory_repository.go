package tenancyport

import "context"

type BackchannelDirectoryRepository interface {
	BackchannelApps(ctx context.Context, tenantID string) (slugs []string, uris []string, err error)
}
