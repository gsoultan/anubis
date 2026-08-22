package repository

import "context"

// BackchannelNotifier delivers signed logout tokens to applications.
type BackchannelNotifier interface {
	NotifyLogout(ctx context.Context, uri string, logoutToken string)
}
