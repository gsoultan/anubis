package snapshot

import "time"

// Evaluate mirrors migrations/0013 authorize() over the frozen data:
//   - identity state + assurance gates (0009 gates 1-2)
//   - candidates: live grants whose role confers the permission
//   - OR within an axis (any granted node covers the target), AND across axes
//   - fail-closed: a constrained axis without a target is unsatisfied; a
//     strict axis a grant does not address denies the grant
//   - self-scope: reserved "_owner" target must equal the subject
//
// The integration suite differentially tests this against the SQL engine on
// seeded data — the two implementations must never disagree.
func (d *Data) Evaluate(identityID, permission string, targets map[string]string, now time.Time) bool {
	ident, ok := d.Identities[identityID]
	if !ok || ident.Blocked {
		return false
	}
	perm, ok := d.Permissions[permission]
	if !ok {
		return false
	}
	if perm.MinAssurance > ident.AssuranceLevel {
		return false
	}

	owner, hasOwner := targets["_owner"]

	for _, g := range d.GrantsByIdentity[identityID] {
		if g.ValidFrom.After(now) {
			continue
		}
		if !g.ValidUntil.IsZero() && !g.ValidUntil.After(now) {
			continue
		}
		if !d.RolePermissions[g.RoleID][permission] {
			continue
		}
		if g.SelfScoped && (!hasOwner || owner != identityID) {
			continue
		}
		if !d.grantScopesSatisfied(g, targets) {
			continue
		}
		if !d.strictAxesAddressed(g) {
			continue
		}
		return true
	}
	return false
}

// grantScopesSatisfied: every constrained axis must have at least one granted
// node covering the target (ancestor-or-self; depth 0 only when inherit=false).
func (d *Data) grantScopesSatisfied(g Grant, targets map[string]string) bool {
	for axis, constraints := range g.Scopes {
		target, supplied := targets[axis]
		if !supplied {
			return false // fail-closed: constrained axis without a target
		}
		up := d.Up[target]
		satisfied := false
		for _, c := range constraints {
			depth, covered := up[c.NodeID]
			if covered && (c.Inherit || depth == 0) {
				satisfied = true
				break
			}
		}
		if !satisfied {
			return false
		}
	}
	return true
}

// strictAxesAddressed: a strict (default_effect=deny) axis denies any grant
// that carries no constraint on it.
func (d *Data) strictAxesAddressed(g Grant) bool {
	for axis := range d.StrictAxes {
		if _, ok := g.Scopes[axis]; !ok {
			return false
		}
	}
	return true
}

// SessionAlive answers the gate's revocation question from the snapshot.
func (d *Data) SessionAlive(sessionID string, epoch int, identityID string) bool {
	if d.RevokedSessions[sessionID] {
		return false
	}
	ident, ok := d.Identities[identityID]
	if !ok || ident.Blocked {
		return false
	}
	return ident.TokenEpoch == epoch
}
