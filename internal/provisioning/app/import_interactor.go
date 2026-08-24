package provisioningapp

import (
	"bytes"
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/guard"
	identityapp "github.com/gsoultan/anubis/internal/identity/app"
	"github.com/gsoultan/anubis/internal/platform/xlsx"
	provisioningdomain "github.com/gsoultan/anubis/internal/provisioning/domain"
	"github.com/gsoultan/anubis/internal/provisioning/domain/row"
	"github.com/gsoultan/anubis/internal/provisioning/domain/schema"
	provisioningport "github.com/gsoultan/anubis/internal/provisioning/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
)

// MaxUploadBytes bounds an uploaded workbook. A twenty thousand row
// spreadsheet is comfortably under a megabyte; anything at this size is
// either not a directory or not something to unpack in memory.
const MaxUploadBytes = 8 << 20

// The import demands only what the sheets it was actually given need. An
// operator who may create people but not grant roles can still upload a
// People-only workbook, and one who may do neither cannot use a dry run to
// ask which roles exist.
const (
	permRead        = "anubis:identity:read"
	permPeopleWrite = "anubis:identity:write"
	// Membership assignment is gated on grant:admin by the authz context
	// itself, so that is what is demanded here — asking for a permission
	// the write will not check would let a caller through to fail later.
	permAccessWrite = "anubis:grant:admin"
)

type importInteractor struct {
	guard  *guard.Guard
	dir    provisioningport.DirectoryReader
	people provisioningport.PeopleWriter
	access provisioningport.AccessReader
	writer provisioningport.AccessWriter
	scope  provisioningport.ScopeReader
	clock  clock.Clock
	tx     txm.TxManager
	audit  auditport.Auditor
}

func NewImportInteractor(
	// ops lets a PLATFORM OPERATOR do this inside a tenant they are assigned
	// to (ADR-0011): their authority is an assignment, not a grant.
	ops guard.OperatorAuthority,
	clockNow func() time.Time,
	dir provisioningport.DirectoryReader,
	people provisioningport.PeopleWriter,
	access provisioningport.AccessReader,
	writer provisioningport.AccessWriter,
	scope provisioningport.ScopeReader,
	clk clock.Clock,
	tx txm.TxManager,
	audit auditport.Auditor,
) ImportUsecase {
	return &importInteractor{
		guard: guard.New().WithOperators(ops, clockNow), dir: dir, people: people, access: access,
		writer: writer, scope: scope, clock: clk, tx: tx, audit: audit,
	}
}

func (u *importInteractor) ImportTemplate(ctx context.Context) ([]byte, error) {
	if _, err := u.guard.Require(ctx, permRead); err != nil {
		return nil, err
	}
	return renderTemplate()
}

// ImportWorkbook validates an entire workbook before it writes anything.
//
// The two phases are the point. Validation resolves every name the file
// uses and collects every failure, so an operator gets the whole list of
// problems in one go instead of one per upload. Writing only starts once
// that list is empty, which also keeps the writes clear of Postgres
// aborting a transaction on its first failed statement — inside one
// transaction, "carry on and report the next problem" produces nothing
// but a cascade of meaningless follow-on errors.
//
// An import therefore applies whole or not at all. A half-imported
// directory is not a state anyone should have to unpick by hand.
func (u *importInteractor) ImportWorkbook(ctx context.Context, in ImportInput) (*provisioningdomain.ImportReport, error) {
	p, err := u.guard.Require(ctx, permRead)
	if err != nil {
		return nil, err
	}
	switch {
	case len(in.Data) == 0:
		return nil, apperr.ErrInvalidArgument.With("file", "no file uploaded")
	case len(in.Data) > MaxUploadBytes:
		return nil, apperr.ErrInvalidArgument.With("file", "larger than the 8 MiB limit")
	}

	sheets, err := xlsx.Read(bytes.NewReader(in.Data), int64(len(in.Data)))
	if err != nil {
		return nil, apperr.ErrInvalidArgument.With("file", "not a readable .xlsx workbook")
	}
	tables := make(map[string]provisioningdomain.Table, len(sheets))
	for _, s := range sheets {
		tables[s.Name] = provisioningdomain.Table{Header: s.Header(), Rows: s.Rows}
	}
	wb, issues := provisioningdomain.Parse(tables)

	if len(wb.People) > 0 {
		if _, err := u.guard.Require(ctx, permPeopleWrite); err != nil {
			return nil, err
		}
	}
	if len(wb.Grants) > 0 || len(wb.Memberships) > 0 {
		if _, err := u.guard.Require(ctx, permAccessWrite); err != nil {
			return nil, err
		}
	}

	report := &provisioningdomain.ImportReport{Dry: in.Dry}
	report.AddIssues(issues)
	if wb.Len() == 0 {
		return report, nil
	}

	res := newResolver(p.TenantID, u.dir, u.access, u.scope)
	if err := u.validate(ctx, res, wb, report); err != nil {
		return nil, err
	}
	if in.Dry || !report.OK() {
		return report, nil
	}

	// Validation already projected the counters, and the apply pass does
	// exactly what it projected or fails outright — so there is nothing
	// left to count here.
	if err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		return u.apply(ctx, res, wb, report)
	}); err != nil {
		return nil, err
	}
	report.Applied = true
	u.emit(ctx, p, report)
	return report, nil
}

// validate resolves every name the workbook uses and projects what the
// import would do. It writes nothing.
func (u *importInteractor) validate(ctx context.Context, res *resolver,
	wb provisioningdomain.Workbook, rep *provisioningdomain.ImportReport) error {

	for _, person := range wb.People {
		_, exists, err := res.identityID(ctx, person.Realm, person.Username)
		if err != nil {
			return err
		}
		if exists {
			rep.PeopleExisting++
			continue
		}
		if _, ok, err := res.realmID(ctx, person.Realm); err != nil {
			return err
		} else if !ok {
			rep.AddIssue(issue(schema.SheetPeople, person.Row, schema.ColRealm, "no realm with this code"))
			continue
		}
		res.expect(person.Realm, person.Username)
		rep.PeopleCreated++
	}

	for _, g := range wb.Grants {
		identityID, ok, err := u.mustFindPerson(ctx, res, schema.SheetGrants, g.Line(), g.Realm, g.Username, rep)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		roleID, found, err := res.roleID(ctx, g.Role)
		if err != nil {
			return err
		}
		if !found {
			rep.AddIssue(issue(schema.SheetGrants, g.Line(), schema.ColRole, "no role with this name"))
			continue
		}
		bad := false
		for _, s := range g.Scopes {
			if _, found, err := res.nodeID(ctx, s.Axis, s.Ref); err != nil {
				return err
			} else if !found {
				rep.AddIssue(issue(schema.SheetGrants, g.Line(), schema.ColScopeRef,
					"no node with this reference on axis "+s.Axis))
				bad = true
			}
		}
		if bad {
			continue
		}
		// Somebody who does not exist yet cannot already hold the role.
		if identityID != "" {
			held, err := u.holdsRole(ctx, res, identityID, roleID)
			if err != nil {
				return err
			}
			if held {
				rep.GrantsSkipped++
				continue
			}
		}
		rep.GrantsCreated++
	}

	for _, m := range wb.Memberships {
		if _, ok, err := u.mustFindPerson(ctx, res, schema.SheetMemberships, m.Row, m.Realm, m.Username, rep); err != nil {
			return err
		} else if !ok {
			continue
		}
		if _, found, err := res.membershipID(ctx, m.Name); err != nil {
			return err
		} else if !found {
			rep.AddIssue(issue(schema.SheetMemberships, m.Row, schema.ColMembership, "no membership with this name"))
			continue
		}
		rep.MembershipsAssigned++
	}
	return nil
}

// apply performs the work validate projected, inside the caller's
// transaction. Any failure aborts the whole import.
func (u *importInteractor) apply(ctx context.Context, res *resolver,
	wb provisioningdomain.Workbook, rep *provisioningdomain.ImportReport) error {
	for _, person := range wb.People {
		if _, exists, err := res.identityID(ctx, person.Realm, person.Username); err != nil {
			return err
		} else if exists {
			continue
		}
		rec, err := u.people.CreateIdentity(ctx, identityapp.AdminCreateIdentity{
			Realm:          person.Realm,
			Username:       person.Username,
			Email:          person.Email,
			Category:       person.Category,
			ExternalRef:    person.ExternalRef,
			AssuranceLevel: person.AssuranceLevel,
		})
		if err != nil {
			return rowErr(schema.SheetPeople, person.Row, err)
		}
		res.remember(person.Realm, person.Username, rec.ID)
	}

	for _, g := range wb.Grants {
		identityID, ok, err := res.identityID(ctx, g.Realm, g.Username)
		if err != nil {
			return err
		}
		if !ok {
			return rowErr(schema.SheetGrants, g.Line(), apperr.ErrNotFound.With("username", g.Username))
		}
		roleID, ok, err := res.roleID(ctx, g.Role)
		if err != nil {
			return err
		}
		if !ok {
			return rowErr(schema.SheetGrants, g.Line(), apperr.ErrNotFound.With("role", g.Role))
		}
		held, err := u.holdsRole(ctx, res, identityID, roleID)
		if err != nil {
			return err
		}
		if held {
			continue
		}
		scopes, err := u.scopeInputs(ctx, res, g)
		if err != nil {
			return err
		}
		// TenantID and GrantedBy are filled in by the authz usecase from
		// the caller's own principal; setting them here would be ignored.
		if _, err := u.writer.CreateGrant(ctx, grant.GrantCreate{
			IdentityID: identityID,
			RoleID:     roleID,
			Reason:     g.Reason,
			ValidUntil: g.ValidUntil,
			Scopes:     scopes,
		}); err != nil {
			return rowErr(schema.SheetGrants, g.Line(), err)
		}
		res.granted(identityID, roleID)
	}

	rep.MembershipsAssigned, rep.MembershipsExisting = 0, 0
	for _, m := range wb.Memberships {
		identityID, ok, err := res.identityID(ctx, m.Realm, m.Username)
		if err != nil {
			return err
		}
		if !ok {
			return rowErr(schema.SheetMemberships, m.Row, apperr.ErrNotFound.With("username", m.Username))
		}
		membershipID, ok, err := res.membershipID(ctx, m.Name)
		if err != nil {
			return err
		}
		if !ok {
			return rowErr(schema.SheetMemberships, m.Row, apperr.ErrNotFound.With("membership", m.Name))
		}
		// AssignMembership is already idempotent and reports how many
		// rows it touched, which is the one thing validation could not
		// know — so the real numbers replace its projection here.
		n, err := u.writer.AssignMembership(ctx, membershipID, identityID)
		if err != nil {
			return rowErr(schema.SheetMemberships, m.Row, err)
		}
		if n == 0 {
			rep.MembershipsExisting++
		} else {
			rep.MembershipsAssigned++
		}
	}
	return nil
}

func (u *importInteractor) scopeInputs(ctx context.Context, res *resolver, g row.Grant) ([]grant.GrantScopeInput, error) {
	if len(g.Scopes) == 0 {
		return nil, nil
	}
	out := make([]grant.GrantScopeInput, 0, len(g.Scopes))
	for _, s := range g.Scopes {
		nodeID, ok, err := res.nodeID(ctx, s.Axis, s.Ref)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, rowErr(schema.SheetGrants, g.Line(), apperr.ErrNotFound.With("scope_ref", s.Ref))
		}
		out = append(out, grant.GrantScopeInput{Axis: s.Axis, NodeID: nodeID, Inherit: s.Inherit})
	}
	return out, nil
}

// mustFindPerson resolves the realm and username a sheet identifies
// someone by, counting somebody the People sheet is about to create as
// found. It returns an empty id in that case, since they have no id yet.
func (u *importInteractor) mustFindPerson(ctx context.Context, res *resolver,
	sheet string, line int, realm, username string,
	rep *provisioningdomain.ImportReport) (string, bool, error) {

	id, exists, err := res.identityID(ctx, realm, username)
	if err != nil {
		return "", false, err
	}
	if exists {
		return id, true, nil
	}
	if res.expected(realm, username) {
		return "", true, nil
	}
	if _, ok, err := res.realmID(ctx, realm); err != nil {
		return "", false, err
	} else if !ok {
		rep.AddIssue(issue(sheet, line, schema.ColRealm, "no realm with this code"))
		return "", false, nil
	}
	rep.AddIssue(issue(sheet, line, schema.ColUsername,
		"nobody by this name in realm "+realm+", and they are not on the People sheet"))
	return "", false, nil
}

// holdsRole reports whether the person already holds a live grant of this
// role.
func (u *importInteractor) holdsRole(ctx context.Context, res *resolver, identityID, roleID string) (bool, error) {
	held, err := res.heldRoles(ctx, identityID, u.clock.Now())
	if err != nil {
		return false, err
	}
	return held[roleID], nil
}

func (u *importInteractor) emit(ctx context.Context, p *authctx.Principal, rep *provisioningdomain.ImportReport) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: p.TenantID,
		Action: "provisioning.import", Result: "allow",
		IP: authctx.ClientIP(ctx),
		Detail: jsonx.Must(map[string]string{
			"people_created":       itoa(rep.PeopleCreated),
			"people_existing":      itoa(rep.PeopleExisting),
			"grants_created":       itoa(rep.GrantsCreated),
			"grants_skipped":       itoa(rep.GrantsSkipped),
			"memberships_assigned": itoa(rep.MembershipsAssigned),
		}),
	})
}

func issue(sheet string, line int, column, message string) provisioningdomain.RowIssue {
	return provisioningdomain.RowIssue{Sheet: sheet, Row: line, Column: column, Message: message}
}

// rowErr keeps the sheet and row on an error that aborts the import, so
// the operator is told which line stopped it rather than just that
// something did.
func rowErr(sheet string, line int, err error) error {
	e := apperr.AsError(err)
	return apperr.ErrInvalidArgument.With(sheet, "row "+itoa(line)+": "+e.Message)
}
