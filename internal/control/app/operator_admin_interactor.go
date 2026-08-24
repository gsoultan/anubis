package controlapp

import (
	"context"
	"strings"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	controlport "github.com/gsoultan/anubis/internal/control/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	"github.com/gsoultan/anubis/internal/shared/validate"
)

// Page bounds. A caller asking for everything gets a page anyway: this is a
// directory, and an endpoint that answers "all of them" is one that stops
// working at exactly the size where the answer starts to matter.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

type operatorAdminInteractor struct {
	guard   platformGuard
	users   controlport.PlatformUserStore
	tenants controlport.TenantLookup
	assign  controlport.AssignmentWriter
	read    controlport.AssignmentReader
	clock   clock.Clock
	tx      txm.TxManager
	audit   auditport.Auditor
}

func NewOperatorAdminInteractor(
	users controlport.PlatformUserStore,
	tenants controlport.TenantLookup,
	assign controlport.AssignmentWriter,
	read controlport.AssignmentReader,
	clk clock.Clock,
	tx txm.TxManager,
	audit auditport.Auditor,
) OperatorAdminUsecase {
	return &operatorAdminInteractor{
		guard: platformGuard{read: read, clock: clk}, users: users, tenants: tenants,
		assign: assign, read: read, clock: clk, tx: tx, audit: audit,
	}
}

func (u *operatorAdminInteractor) ListOperators(ctx context.Context, in ListOperatorsInput) (*controldomain.Page, error) {
	// Any operator may see who else operates the installation; changing that
	// list is what needs the permission.
	if _, err := u.guard.requireAny(ctx); err != nil {
		return nil, err
	}
	size := in.PageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}

	// One extra row answers "is there another page" without a second query
	// and without COUNT-ing the filtered set.
	users, err := u.users.ListPlatformUsers(ctx, strings.TrimSpace(in.Query), in.Cursor, int32(size+1))
	if err != nil {
		return nil, err
	}
	page := &controldomain.Page{}
	if len(users) > size {
		page.NextCursor = users[size-1].Username
		users = users[:size]
	}
	if total, terr := u.users.CountPlatformUsers(ctx); terr == nil {
		page.Total = total
	}

	assignments, err := u.read.Assignments(ctx)
	if err != nil {
		return nil, err
	}
	byOperator := make(map[string][]controldomain.AssignmentRecord, len(assignments))
	for _, a := range assignments {
		byOperator[a.OperatorID] = append(byOperator[a.OperatorID], a)
	}
	for i := range users {
		users[i].Assignments = byOperator[users[i].ID]
	}
	page.Users = users
	return page, nil
}

// CreateOperator adds a platform administrator.
//
// One transaction: an operator that exists without the authority it was
// created with is an account nobody looks at and nobody thinks to remove.
func (u *operatorAdminInteractor) CreateOperator(ctx context.Context, in CreateOperatorInput) (string, string, error) {
	p, _, err := u.guard.require(ctx, controldomain.PermAssignOperators)
	if err != nil {
		return "", "", err
	}
	if !in.Role.Valid() {
		return "", "", apperr.ErrInvalidArgument.With("role", string(in.Role))
	}
	username := strings.TrimSpace(in.Username)
	if !validate.ValidUsername(username) {
		return "", "", apperr.ErrInvalidArgument.With("username", "letters, digits, dot, dash or underscore")
	}
	// A platform user belongs to no realm, so there is no realm policy to
	// consult — this is the one place the rule is stated for them.
	if len([]rune(in.Password)) < controldomain.MinOwnerPassword {
		return "", "", apperr.ErrInvalidArgument.With("password", "at least 12 characters")
	}
	if existing, _, lerr := u.users.PlatformUserByUsername(ctx, username); lerr == nil && existing != nil {
		return "", "", apperr.ErrInvalidArgument.With("username", "already taken")
	}

	tenantID, target, err := u.resolveTenant(ctx, in.TenantSlug)
	if err != nil {
		return "", "", err
	}

	var operatorID, assignmentID string
	if err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		hash, herr := kdf.Hash(in.Password)
		if herr != nil {
			return herr
		}
		id, cerr := u.users.CreatePlatformUser(ctx, username, strings.TrimSpace(in.Email), hash)
		if cerr != nil {
			return cerr
		}
		operatorID = id
		aid, aerr := u.assign.CreateAssignment(ctx, controldomain.AssignmentRecord{
			OperatorID: id, TenantID: tenantID, Role: in.Role,
			GrantedBy: p.IdentityID, Reason: in.Reason,
		})
		if aerr != nil {
			return aerr
		}
		assignmentID = aid
		return nil
	}); err != nil {
		return "", "", err
	}

	u.emit(ctx, p, "platform.operator_create", operatorID, map[string]string{
		"username": username, "tenant": target, "role": string(in.Role),
	})
	return operatorID, assignmentID, nil
}

func (u *operatorAdminInteractor) AssignOperator(ctx context.Context, in AssignOperatorInput) (string, error) {
	p, _, err := u.guard.require(ctx, controldomain.PermAssignOperators)
	if err != nil {
		return "", err
	}
	if !in.Role.Valid() {
		return "", apperr.ErrInvalidArgument.With("role", string(in.Role))
	}
	operatorID := in.OperatorID
	if operatorID == "" {
		who, _, lerr := u.users.PlatformUserByUsername(ctx, strings.TrimSpace(in.OperatorUsername))
		if lerr != nil {
			return "", lerr
		}
		if who == nil {
			return "", apperr.ErrInvalidArgument.With("operator",
				"no platform user by that name — create the account first")
		}
		operatorID = who.ID
	}
	tenantID, target, err := u.resolveTenant(ctx, in.TenantSlug)
	if err != nil {
		return "", err
	}
	id, err := u.assign.CreateAssignment(ctx, controldomain.AssignmentRecord{
		OperatorID: operatorID, TenantID: tenantID, Role: in.Role,
		GrantedBy: p.IdentityID, Reason: in.Reason, ValidUntil: in.ValidUntil,
	})
	if err != nil {
		return "", err
	}
	u.emit(ctx, p, "platform.operator_assign", operatorID, map[string]string{
		"assignment_id": id, "tenant": target, "role": string(in.Role),
	})
	return id, nil
}

func (u *operatorAdminInteractor) RevokeAssignment(ctx context.Context, assignmentID string) error {
	p, _, err := u.guard.require(ctx, controldomain.PermAssignOperators)
	if err != nil {
		return err
	}
	if err := u.lastOwnerCheck(ctx, assignmentID); err != nil {
		return err
	}
	if err := u.read.RevokeAssignment(ctx, assignmentID); err != nil {
		return err
	}
	u.emit(ctx, p, "platform.operator_revoke", assignmentID, nil)
	return nil
}

// SetOperatorStatus disables or restores an operator.
//
// Disabling is not the same as revoking their assignments: the assignments
// survive, so restoring somebody puts back exactly what they had rather than
// leaving whoever restores them to reconstruct it from memory.
func (u *operatorAdminInteractor) SetOperatorStatus(ctx context.Context, operatorID, status string) error {
	p, _, err := u.guard.require(ctx, controldomain.PermAssignOperators)
	if err != nil {
		return err
	}
	switch status {
	case "active", "disabled":
	default:
		return apperr.ErrInvalidArgument.With("status", status)
	}
	if status == "disabled" && operatorID == p.IdentityID {
		// Locking yourself out of the console you administer helps nobody.
		return apperr.ErrInvalidArgument.With("operator", "you cannot disable your own account")
	}
	if err := u.users.SetStatus(ctx, operatorID, status); err != nil {
		return err
	}
	u.emit(ctx, p, "platform.operator_status", operatorID, map[string]string{"status": status})
	return nil
}

// resolveTenant turns a slug into an id. An empty slug means every tenant,
// which is the installation owner and not an error.
func (u *operatorAdminInteractor) resolveTenant(ctx context.Context, slug string) (string, string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", "*", nil
	}
	t, err := u.tenants.TenantBySlug(ctx, slug)
	if err != nil || t == nil {
		return "", "", apperr.ErrNotFound.With("tenant", slug)
	}
	return t.ID, t.Slug, nil
}

// lastOwnerCheck refuses to remove the final owner. An installation with no
// owner has nobody who can appoint one, and the only way back is the database.
func (u *operatorAdminInteractor) lastOwnerCheck(ctx context.Context, assignmentID string) error {
	all, err := u.read.Assignments(ctx)
	if err != nil {
		return err
	}
	now := u.clock.Now()
	owners, isOwner := 0, false
	for _, a := range all {
		if a.Global() && a.Role == controldomain.RoleOwner && a.Live(now) {
			owners++
			if a.ID == assignmentID {
				isOwner = true
			}
		}
	}
	if isOwner && owners <= 1 {
		return apperr.ErrInvalidArgument.With("assignment",
			"this is the only owner — appoint another before removing this one")
	}
	return nil
}

func (u *operatorAdminInteractor) emit(ctx context.Context, p *authctx.Principal, action, target string, detail map[string]string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		// ActorKind distinguishes the two populations in the log: reading
		// "identity" for somebody who is not one would misattribute the act.
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "platform_user",
		SessionID: p.SessionID, TargetID: target, Action: action, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(detail),
	})
}
