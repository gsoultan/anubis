// Package endpoint wraps services in go-kit endpoints. This is the ONLY
// layer where cross-cutting middleware lives: recover, request-id, logging,
// metrics, rate limiting. Transports call endpoints; endpoints call
// services; nothing here touches business rules or wire formats.
package mw
