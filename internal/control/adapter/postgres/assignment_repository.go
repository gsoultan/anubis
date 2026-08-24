package controlpg

import (
	"context"

	"github.com/gsoultan/anubis/internal/control/adapter/postgres/gen"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// CreateAssignment records operator authority. An empty tenantID means every
// tenant — the installation owner.
func (s *Repository) CreateAssignment(ctx context.Context, a controldomain.AssignmentRecord) (string, error) {
	arg := gen.CreatePlatformAssignmentParams{
		OperatorID: a.OperatorID,
		Role:       string(a.Role),
		Reason:     a.Reason,
	}
	if a.TenantID != "" {
		arg.TenantID = &a.TenantID
	}
	if a.GrantedBy != "" {
		arg.GrantedBy = &a.GrantedBy
	}
	arg.ValidUntil = a.ValidUntil
	id, err := s.q(ctx).CreatePlatformAssignment(ctx, arg)
	if err != nil {
		return "", database.MapErr(err)
	}
	return id, nil
}

// AssignmentsForOperator is the guard's lookup: everything still in force.
func (s *Repository) AssignmentsForOperator(ctx context.Context, operatorID string) ([]controldomain.AssignmentRecord, error) {
	rows, err := s.q(ctx).ListAssignmentsForOperator(ctx, operatorID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]controldomain.AssignmentRecord, 0, len(rows))
	for _, r := range rows {
		a := controldomain.AssignmentRecord{
			ID: r.ID, OperatorID: r.OperatorID, Role: controldomain.OperatorRole(r.Role),
			Reason: r.Reason, ValidUntil: r.ValidUntil, RevokedAt: r.RevokedAt,
			CreatedAt: r.CreatedAt,
		}
		if r.TenantID != nil {
			a.TenantID = *r.TenantID
		}
		if r.GrantedBy != nil {
			a.GrantedBy = *r.GrantedBy
		}
		out = append(out, a)
	}
	return out, nil
}

// HasOwner reports whether the installation already has an owner, which is
// what setup checks before agreeing to create one.
func (s *Repository) HasOwner(ctx context.Context) (bool, error) {
	present, err := s.q(ctx).HasAnyPlatformOwner(ctx)
	if err != nil {
		return false, database.MapErr(err)
	}
	return present, nil
}

// Assignments is every live assignment in the installation, with the tenant
// slug resolved for display.
func (s *Repository) Assignments(ctx context.Context) ([]controldomain.AssignmentRecord, error) {
	rows, err := s.q(ctx).ListAssignments(ctx)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]controldomain.AssignmentRecord, 0, len(rows))
	for _, r := range rows {
		a := controldomain.AssignmentRecord{
			ID: r.ID, OperatorID: r.OperatorID, Role: controldomain.OperatorRole(r.Role),
			Reason: r.Reason, ValidUntil: r.ValidUntil, RevokedAt: r.RevokedAt,
			CreatedAt: r.CreatedAt,
		}
		if r.TenantID != nil {
			a.TenantID = *r.TenantID
		}
		if r.TenantSlug != nil {
			a.TenantSlug = *r.TenantSlug
		}
		out = append(out, a)
	}
	return out, nil
}

// RevokeAssignment ends one operator's authority. Revoking is preferred to
// deleting: the row is the record that the authority once existed, which is
// what an audit of "who could reach this tenant in March" depends on.
func (s *Repository) RevokeAssignment(ctx context.Context, id string) error {
	n, err := s.q(ctx).RevokeAssignment(ctx, id)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("assignment", id)
	}
	return nil
}
