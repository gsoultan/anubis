package provisioningapp

import (
	"context"
	"strings"
	"time"

	provisioningport "github.com/gsoultan/anubis/internal/provisioning/port"
)

// resolver turns the names a workbook uses — realm codes, usernames, role
// names, membership names, scope node references — into ids, and
// remembers both the hits and the misses.
//
// The caching is not a micro-optimisation. A four thousand row sheet
// granting one role names that role four thousand times, and without the
// negative half a workbook full of typos would hammer the database once
// per bad row.
//
// Every map is keyed by values out of the uploaded file, and every one is
// bounded by the per-sheet row limit the domain enforces before any of
// this runs.
type resolver struct {
	tenantID string
	dir      provisioningport.DirectoryReader
	access   provisioningport.AccessReader
	scope    provisioningport.ScopeReader

	realms      map[string]string
	identities  map[string]string
	roles       map[string]string
	nodes       map[string]string
	memberships map[string]string
	loadedMems  bool

	// held is the roles each person already holds, cached per identity:
	// a sheet granting one role to four thousand people would otherwise
	// ask the same question once per row.
	held map[string]map[string]bool

	// pending is the people the People sheet is about to create. The
	// Grants and Memberships sheets routinely name somebody who does not
	// exist yet — that is the whole point of importing them together — so
	// validation has to treat "will exist" as good enough.
	pending map[string]bool
}

func newResolver(tenantID string, dir provisioningport.DirectoryReader,
	access provisioningport.AccessReader, scope provisioningport.ScopeReader) *resolver {
	return &resolver{
		tenantID: tenantID, dir: dir, access: access, scope: scope,
		realms:     map[string]string{},
		identities: map[string]string{},
		roles:      map[string]string{},
		nodes:      map[string]string{},
		held:       map[string]map[string]bool{},
		pending:    map[string]bool{},
	}
}

// An empty string cached against a key means "looked this up and it is not
// there", which is why every lookup returns found separately from the id.

func (r *resolver) realmID(ctx context.Context, code string) (string, bool, error) {
	if id, cached := r.realms[code]; cached {
		return id, id != "", nil
	}
	realm, err := r.dir.RealmByCode(ctx, r.tenantID, code)
	if err != nil || realm == nil {
		// A miss and a lookup failure are indistinguishable here by
		// design: RealmByCode reports "no such realm" as an error.
		r.realms[code] = ""
		return "", false, nil
	}
	r.realms[code] = realm.ID
	return realm.ID, true, nil
}

func (r *resolver) identityID(ctx context.Context, realmCode, username string) (string, bool, error) {
	key := realmCode + "\x00" + username
	if id, cached := r.identities[key]; cached {
		return id, id != "", nil
	}
	realmID, ok, err := r.realmID(ctx, realmCode)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	who, err := r.dir.IdentityForLogin(ctx, r.tenantID, realmID, username)
	if err != nil || who == nil {
		r.identities[key] = ""
		return "", false, nil
	}
	r.identities[key] = who.ID
	return who.ID, true, nil
}

// remember records an identity the import has just created, so the Grants
// and Memberships sheets can refer to somebody the People sheet only
// brought into existence a moment ago.
func (r *resolver) remember(realmCode, username, id string) {
	r.identities[realmCode+"\x00"+username] = id
}

// expect notes that the People sheet will create this person.
func (r *resolver) expect(realmCode, username string) {
	r.pending[realmCode+"\x00"+username] = true
}

// expected reports whether the People sheet is bringing this person into
// existence in the same import.
func (r *resolver) expected(realmCode, username string) bool {
	return r.pending[realmCode+"\x00"+username]
}

func (r *resolver) roleID(ctx context.Context, name string) (string, bool, error) {
	if id, cached := r.roles[name]; cached {
		return id, id != "", nil
	}
	role, err := r.access.RoleByName(ctx, r.tenantID, name)
	if err != nil || role == nil {
		r.roles[name] = ""
		return "", false, nil
	}
	r.roles[name] = role.ID
	return role.ID, true, nil
}

func (r *resolver) nodeID(ctx context.Context, axis, ref string) (string, bool, error) {
	key := axis + "\x00" + ref
	if id, cached := r.nodes[key]; cached {
		return id, id != "", nil
	}
	node, err := r.scope.ScopeNodeByRef(ctx, r.tenantID, axis, ref)
	if err != nil || node == nil {
		r.nodes[key] = ""
		return "", false, nil
	}
	r.nodes[key] = node.ID
	return node.ID, true, nil
}

// membershipID matches on name, case-insensitively. Memberships have no
// by-name lookup of their own, so the whole list is fetched once — there
// are tens of them, not thousands.
func (r *resolver) membershipID(ctx context.Context, name string) (string, bool, error) {
	if !r.loadedMems {
		ms, err := r.access.ListMemberships(ctx, r.tenantID)
		if err != nil {
			return "", false, err
		}
		r.memberships = make(map[string]string, len(ms))
		for _, m := range ms {
			r.memberships[strings.ToLower(m.Name)] = m.ID
		}
		r.loadedMems = true
	}
	id, ok := r.memberships[strings.ToLower(name)]
	return id, ok, nil
}

// heldRoles is the set of roles this person already holds under a live
// grant. It is what makes re-running an import a no-op rather than a
// second pile of duplicate grants.
func (r *resolver) heldRoles(ctx context.Context, identityID string, now time.Time) (map[string]bool, error) {
	if cached, ok := r.held[identityID]; ok {
		return cached, nil
	}
	grants, err := r.access.ListGrants(ctx, r.tenantID, identityID, false)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(grants))
	for _, g := range grants {
		if g.RevokedAt != nil {
			continue
		}
		if g.ValidUntil != nil && !g.ValidUntil.After(now) {
			continue
		}
		out[g.RoleID] = true
	}
	r.held[identityID] = out
	return out, nil
}

// granted notes a role just granted, so a workbook that grants the same
// role to the same person on two separate rows does it once.
func (r *resolver) granted(identityID, roleID string) {
	if set, ok := r.held[identityID]; ok {
		set[roleID] = true
		return
	}
	r.held[identityID] = map[string]bool{roleID: true}
}
