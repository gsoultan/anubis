//go:build integration

package integration

import (
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
)

// Test doubles for the collaborators the timing probe does not exercise:
// the wrong-password path never reaches token issuance, and audit writes
// would add database latency to a measurement about KDF cost.

type nopIssuer struct{}

func (nopIssuer) Issue(context.Context, authapp.IssueInput) (*authapp.TokenPair, error) {
	return &authapp.TokenPair{}, nil
}

type nopAuditor struct{}

func (nopAuditor) Emit(context.Context, auditdomain.AuditEvent) {}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
