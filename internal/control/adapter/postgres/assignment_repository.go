package controlpg

import (
	"context"

	"github.com/gsoultan/anubis/internal/control/adapter/postgres/rgen/platformassignment"
	controlrquery "github.com/gsoultan/anubis/internal/control/adapter/postgres/rquery"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// CreateAssignment records operator authority. An empty tenantID means every
// tenant — the installation owner.
func (s *Repository) CreateAssignment(ctx context.Context, a controldomain.AssignmentRecord) (string, error) {
	operator, err := database.ParseUUID(a.OperatorID)
	if err != nil {
		return "", database.MapErr(err)
	}
	n := platformassignment.Create()
	n.SetOperatorID(operator)
	n.SetRole(string(a.Role))
	n.SetReason(a.Reason)

	// NULL tenant is the GLOBAL assignment, not a missing one. The partial
	// unique index in the model relies on that distinction, so writing a zero
	// uuid here would quietly create a second kind of row.
	if a.TenantID == "" {
		n.SetTenantIDNull()
	} else {
		tid, err := database.ParseUUID(a.TenantID)
		if err != nil {
			return "", database.MapErr(err)
		}
		n.SetTenantID(tid)
	}
	if a.GrantedBy == "" {
		n.SetGrantedByNull()
	} else {
		by, err := database.ParseUUID(a.GrantedBy)
		if err != nil {
			return "", database.MapErr(err)
		}
		n.SetGrantedBy(by)
	}
	if a.ValidUntil != nil {
		n.SetValidUntil(*a.ValidUntil)
	}
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

// AssignmentsForOperator is the guard's lookup: everything still in force.
func (s *Repository) AssignmentsForOperator(ctx context.Context, operatorID string) ([]controldomain.AssignmentRecord, error) {
	rows, err := controlrquery.ListAssignmentsForOperator.Query(ctx, s.ex(ctx), operatorID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return assignments(rows), nil
}

// Assignments is every live assignment in the installation, with the tenant
// slug resolved for display.
func (s *Repository) Assignments(ctx context.Context) ([]controldomain.AssignmentRecord, error) {
	rows, err := controlrquery.ListAssignments.Query(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	return assignments(rows), nil
}

// assignments maps the shared row shape onto the domain type. The queries
// COALESCE the nullable ids to ”, so an absent tenant and an absent granter
// arrive already in the form the domain uses.
func assignments(rows []controlrquery.AssignmentRow) []controldomain.AssignmentRecord {
	out := make([]controldomain.AssignmentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, controldomain.AssignmentRecord{
			ID: r.ID, OperatorID: r.OperatorID, TenantID: r.TenantID,
			TenantSlug: r.TenantSlug, Role: controldomain.OperatorRole(r.Role),
			GrantedBy: r.GrantedBy, Reason: r.Reason,
			ValidUntil: tptr(r.ValidUntil), RevokedAt: tptr(r.RevokedAt),
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}

// HasOwner reports whether the installation already has an owner, which is
// what setup checks before agreeing to create one.
func (s *Repository) HasOwner(ctx context.Context) (bool, error) {
	row, _, err := controlrquery.HasAnyPlatformOwner.One(ctx, s.ex(ctx))
	if err != nil {
		return false, database.MapErr(err)
	}
	return row.Present, nil
}

// RevokeAssignment ends one operator's authority. Revoking is preferred to
// deleting: the row is the record that the authority once existed, which is
// what an audit of "who could reach this tenant in March" depends on.
func (s *Repository) RevokeAssignment(ctx context.Context, id string) error {
	n, err := controlrquery.RevokeAssignment.Exec(ctx, s.ex(ctx), id)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("assignment", id)
	}
	return nil
}
