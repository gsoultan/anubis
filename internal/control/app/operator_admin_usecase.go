package controlapp

import (
	"context"
	"time"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
)

// OperatorAdminUsecase manages the people who operate this installation.
type OperatorAdminUsecase interface {
	// ListOperators is one page of platform users with the assignments that
	// say which tenants each may administer. Paged because this is a
	// directory, and a screen that silently showed the first N of it would
	// answer "who can administer this installation" wrongly.
	ListOperators(ctx context.Context, in ListOperatorsInput) (*controldomain.Page, error)
	// CreateOperator adds a platform administrator. They are created in the
	// platform user table, which is a separate population from any tenant's
	// own people — a tenant's user is not an operator and cannot become one.
	CreateOperator(ctx context.Context, in CreateOperatorInput) (operatorID, assignmentID string, err error)
	// AssignOperator gives authority over one tenant, or over every tenant
	// when TenantSlug is empty.
	AssignOperator(ctx context.Context, in AssignOperatorInput) (string, error)
	RevokeAssignment(ctx context.Context, assignmentID string) error
	// SetOperatorStatus disables or restores a platform user. Disabling
	// takes their live tokens down with them.
	SetOperatorStatus(ctx context.Context, operatorID, status string) error
}

// ListOperatorsInput is one page request.
type ListOperatorsInput struct {
	Query string
	// Cursor is the last username of the previous page, empty for the first.
	Cursor   string
	PageSize int
}

// CreateOperatorInput is a new platform administrator.
type CreateOperatorInput struct {
	Username string
	Email    string
	Password string

	// The authority they start with. Required: an operator with no
	// assignment can sign in and administer nothing.
	TenantSlug string
	Role       controldomain.OperatorRole
	Reason     string
}

// AssignOperatorInput is one grant of operator authority.
type AssignOperatorInput struct {
	// One of these identifies the operator; a username is what somebody
	// doing the assigning actually has to hand.
	OperatorID       string
	OperatorUsername string
	// TenantSlug empty means every tenant: the installation owner.
	TenantSlug string
	Role       controldomain.OperatorRole
	Reason     string
	ValidUntil *time.Time
}
