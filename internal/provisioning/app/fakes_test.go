package provisioningapp

import (
	"context"
	"errors"
	"time"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	identityapp "github.com/gsoultan/anubis/internal/identity/app"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

var errNotFound = errors.New("not found")

// fakeDirectory is the identity side: which realms exist and who is in them.
type fakeDirectory struct {
	realms   map[string]string // code -> id
	people   map[string]string // realmID\x00username -> identity id
	realmErr error
}

func (f *fakeDirectory) RealmByCode(_ context.Context, _, code string) (*identitydomain.Realm, error) {
	if f.realmErr != nil {
		return nil, f.realmErr
	}
	id, ok := f.realms[code]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return &identitydomain.Realm{ID: id, Code: code}, nil
}

func (f *fakeDirectory) IdentityForLogin(_ context.Context, _, realmID, username string) (*identitydomain.Identity, error) {
	id, ok := f.people[realmID+"\x00"+username]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return &identitydomain.Identity{ID: id, Username: username, RealmID: realmID}, nil
}

// fakePeople records the identities the import creates.
type fakePeople struct {
	created []identityapp.AdminCreateIdentity
	err     error
	next    int
}

func (f *fakePeople) CreateIdentity(_ context.Context, in identityapp.AdminCreateIdentity) (*identitydomain.IdentityRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = append(f.created, in)
	f.next++
	return &identitydomain.IdentityRecord{
		ID: "new-" + in.Username, Username: in.Username, RealmCode: in.Realm,
	}, nil
}

// fakeAccess is the authz read side.
type fakeAccess struct {
	roles       map[string]string // name -> id
	memberships []membership.MembershipRecord
	grants      map[string][]grant.GrantRecord // identity id -> grants
	listCalls   int
}

func (f *fakeAccess) RoleByName(_ context.Context, _, name string) (*authzdomain.RoleRecord, error) {
	id, ok := f.roles[name]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return &authzdomain.RoleRecord{ID: id, Name: name}, nil
}

func (f *fakeAccess) ListMemberships(context.Context, string) ([]membership.MembershipRecord, error) {
	return f.memberships, nil
}

func (f *fakeAccess) ListGrants(_ context.Context, _, identityID string, _ bool) ([]grant.GrantRecord, error) {
	f.listCalls++
	return f.grants[identityID], nil
}

// fakeAccessWriter records the access the import hands out.
type fakeAccessWriter struct {
	grants   []grant.GrantCreate
	assigned [][2]string // membershipID, identityID
	grantErr error
}

func (f *fakeAccessWriter) CreateGrant(_ context.Context, in grant.GrantCreate) (string, error) {
	if f.grantErr != nil {
		return "", f.grantErr
	}
	f.grants = append(f.grants, in)
	return "grant-id", nil
}

func (f *fakeAccessWriter) AssignMembership(_ context.Context, membershipID, identityID string) (int, error) {
	f.assigned = append(f.assigned, [2]string{membershipID, identityID})
	return 1, nil
}

// fakeScope resolves scope node references.
type fakeScope struct{ nodes map[string]string } // axis\x00ref -> id

func (f *fakeScope) ScopeNodeByRef(_ context.Context, _, axis, ref string) (*scopedomain.ScopeNodeRecord, error) {
	id, ok := f.nodes[axis+"\x00"+ref]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return &scopedomain.ScopeNodeRecord{ID: id, Axis: axis, ExternalRef: ref}, nil
}

// fakeTx runs the closure and records whether it was entered at all, which
// is how the tests tell a dry run from a real one.
type fakeTx struct{ entered int }

func (f *fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	f.entered++
	return fn(ctx)
}

type fakeAudit struct{ events []auditdomain.AuditEvent }

func (f *fakeAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	f.events = append(f.events, ev)
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// fakeOperators is the control plane: which assignments the caller holds.
// Administration is operator-only, so this is the ONLY source of authority
// the import tests have — there is no tenant-side permission to fake.
type fakeOperators struct {
	rows []controldomain.AssignmentRecord
}

func (f fakeOperators) AssignmentsForOperator(context.Context, string) ([]controldomain.AssignmentRecord, error) {
	return f.rows, nil
}
