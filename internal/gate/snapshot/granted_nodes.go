package snapshot

import (
	"sort"
	"strings"
	"time"

	"github.com/gsoultan/anubis/internal/authz/domain/granted"
)

// GrantedNodes mirrors Evaluate with one axis left open: every gate Evaluate
// applies — identity state, assurance, validity, role, strict axes, the
// fixed axes — and instead of a verdict on one target it reports what the
// surviving grants say about the open axis. A grant Evaluate would reject
// for any target contributes nothing here, so the list can never name a
// place Evaluate would deny.
//
// known is false when the identity or the permission is not in this
// snapshot — the caller distinguishes "nowhere" from "no such thing".
func (d *Data) GrantedNodes(identityID, permission, axis string, fixed map[string]string, now time.Time) (out granted.Nodes, known bool) {
	out.AsOf = d.LoadedAt
	ident, ok := d.Identities[identityID]
	if !ok {
		return out, false
	}
	perm, ok := d.Permissions[permission]
	if !ok {
		return out, false
	}
	if ident.Blocked || perm.MinAssurance > ident.AssuranceLevel {
		return out, true
	}

	seen := map[string]bool{}
	for _, g := range d.GrantsByIdentity[identityID] {
		if !d.grantQualifies(g, permission, now) {
			continue
		}
		if g.SelfScoped {
			out.OwnRecords = true
			continue
		}
		if !d.otherAxesSatisfied(g, axis, fixed) {
			continue
		}
		cs, limited := g.Scopes[axis]
		if !limited {
			out.Everywhere = true
			continue
		}
		for _, n := range constraintsToNodes(cs) {
			k := nodeKey(n)
			if seen[k] {
				continue
			}
			seen[k] = true
			out.Nodes = append(out.Nodes, n)
		}
	}
	if out.Everywhere {
		// No list at all: a list beside "everywhere" invites an application
		// to filter by it anyway and show less than the grant allows.
		out.Nodes = nil
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return nodeKey(out.Nodes[i]) < nodeKey(out.Nodes[j]) })
	return out, true
}

// Reaches reports whether any live grant — any role, any permission — puts
// the subject at nodeID on axis: named directly, inherited from above, or
// unlimited on that axis. It is the entitlement check behind SwitchScope,
// which pins a session to a place; the permission question is asked later,
// per call, by Evaluate.
func (d *Data) Reaches(identityID, axis, nodeID string, now time.Time) bool {
	ident, ok := d.Identities[identityID]
	if !ok || ident.Blocked {
		return false
	}
	target, known := d.Scope.Resolve(nodeID)
	if !known {
		return false
	}
	for _, g := range d.GrantsByIdentity[identityID] {
		if !g.live(now) || g.SelfScoped || !d.strictAxesAddressed(g) {
			continue
		}
		cs, limited := g.Scopes[axis]
		if !limited {
			return true
		}
		if d.Scope.CoveredBy(target, cs) {
			return true
		}
	}
	return false
}

func (g Grant) live(now time.Time) bool {
	if g.ValidFrom.After(now) {
		return false
	}
	return g.ValidUntil.IsZero() || g.ValidUntil.After(now)
}

// grantQualifies is Evaluate's per-grant gate minus the scope question.
func (d *Data) grantQualifies(g Grant, permission string, now time.Time) bool {
	return g.live(now) && d.RolePermissions[g.RoleID][permission] && d.strictAxesAddressed(g)
}

// otherAxesSatisfied applies the grant's limits on every axis except the
// open one, against the targets the caller fixed. Fail-closed, as Evaluate:
// a limited axis the caller did not fix drops the grant.
func (d *Data) otherAxesSatisfied(g Grant, open string, fixed map[string]string) bool {
	for axis, cs := range g.Scopes {
		if axis == open {
			continue
		}
		target, supplied := fixed[axis]
		if !supplied {
			return false
		}
		node, known := d.Scope.Resolve(target)
		if !known || !d.Scope.CoveredBy(node, cs) {
			return false
		}
	}
	return true
}

// constraintsToNodes turns one grant's constraints on an axis into places:
// every include becomes a node, and every exclude on the same grant hangs
// off each of them — an exclusion vetoes the whole axis for that grant.
func constraintsToNodes(cs []ScopeConstraint) []granted.Node {
	var except []granted.Node
	for _, c := range cs {
		if c.Exclude {
			except = append(except, granted.Node{NodeID: c.NodeID, Inherit: c.Inherit})
		}
	}
	sort.Slice(except, func(i, j int) bool { return nodeKey(except[i]) < nodeKey(except[j]) })
	var nodes []granted.Node
	for _, c := range cs {
		if c.Exclude {
			continue
		}
		n := granted.Node{NodeID: c.NodeID, Inherit: c.Inherit}
		if len(except) > 0 {
			n.Except = append([]granted.Node(nil), except...)
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// nodeKey is a stable identity for de-duplication and ordering: the same
// place with the same reach and the same exceptions is the same answer,
// whichever grant said it.
func nodeKey(n granted.Node) string {
	var b strings.Builder
	b.WriteString(n.NodeID)
	if n.Inherit {
		b.WriteString("+")
	}
	for _, e := range n.Except {
		b.WriteString("|-")
		b.WriteString(e.NodeID)
		if e.Inherit {
			b.WriteString("+")
		}
	}
	return b.String()
}
