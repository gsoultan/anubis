package domain

// The stable error vocabulary.
var (
	ErrInternal = E(KindInternal, "internal", "Internal error")

	// Authentication. Note: invalid_credentials is deliberately used for BOTH
	// unknown-user and wrong-password — never reveal which (user enumeration).
	ErrInvalidCredentials = E(KindUnauthenticated, "invalid_credentials", "Invalid username or password")
	ErrIdentityLocked     = E(KindUnauthenticated, "account_locked", "Account is locked")
	ErrIdentityDisabled   = E(KindUnauthenticated, "account_disabled", "Account is disabled")
	ErrMfaRequired        = E(KindUnauthenticated, "mfa_required", "Second factor required")
	ErrMfaInvalid         = E(KindUnauthenticated, "mfa_invalid", "Invalid or expired second factor")
	ErrTokenInvalid       = E(KindUnauthenticated, "invalid_token", "Invalid or expired token")
	ErrRefreshInvalid     = E(KindUnauthenticated, "invalid_refresh_token", "Invalid or expired refresh token")
	ErrRefreshReuse       = E(KindUnauthenticated, "refresh_token_reuse_detected", "Token family revoked. Re-authentication required.")
	ErrSessionRevoked     = E(KindUnauthenticated, "session_revoked", "Session has been revoked")
	ErrUnauthenticated    = E(KindUnauthenticated, "unauthenticated", "Authentication required")
	ErrDeviceChallenge    = E(KindUnauthenticated, "device_challenge_invalid", "Invalid or expired device challenge")

	// Authorization.
	ErrPermissionDenied = E(KindPermissionDenied, "permission_denied", "Permission denied")
	ErrStepUpRequired   = E(KindPermissionDenied, "step_up_required", "Step-up authentication required")

	// Requests.
	ErrInvalidArgument = E(KindInvalidArgument, "invalid_argument", "Invalid argument")
	ErrNotFound        = E(KindNotFound, "not_found", "Not found")
	ErrConflict        = E(KindConflict, "conflict", "Already exists")
	ErrRateLimited     = E(KindRateLimited, "rate_limited", "Too many requests")

	// External structure feeds (scope sync).
	ErrUnavailableFeed = E(KindUnavailable, "feed_unavailable", "Structure feed is unreachable")

	// Registration / flows.
	ErrRegistrationClosed = E(KindPermissionDenied, "registration_closed", "Self-registration is not enabled for this realm")
	ErrRedirectURI        = E(KindInvalidArgument, "invalid_redirect_uri", "redirect_uri is not registered for this client")
	ErrPKCE               = E(KindUnauthenticated, "invalid_pkce", "PKCE verification failed")
	ErrPasswordPolicy     = E(KindInvalidArgument, "password_policy", "Password does not meet the realm policy")
)
