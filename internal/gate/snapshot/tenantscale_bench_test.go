package snapshot

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// THE CEILING THIS SYSTEM ACTUALLY HAS.
//
// ADR-0015 moved the scale question from tenant SIZE to tenant COUNT: the
// gate is share-nothing and every instance holds every tenant's snapshot, so
// memory is (tenants x per-tenant cost) and nothing about serving one tenant
// tells you what a thousand cost. BenchmarkScopeIndexMemory measures the
// scope index; this measures a WHOLE snapshot, because the index is only one
// of seven maps in Data and the others scale with identities and roles rather
// than with nodes.
//
// The roadmap says lazy per-tenant loading is worth doing only with numbers
// saying sharding is not enough. This produces those numbers.
func BenchmarkSnapshotMemoryPerTenant(b *testing.B) {
	for _, sz := range []struct {
		name                            string
		nodes, identities, roles, perms int
	}{
		// A small tenant: a department, one axis, a handful of roles.
		{"small", 500, 200, 10, 50},
		// A typical mid-size directory.
		{"medium", 5_000, 5_000, 50, 200},
		// A large one. Not the 1M-node ceiling — that is a single-tenant
		// question ADR-0015 already answers — but a big real customer.
		{"large", 50_000, 50_000, 200, 500},
	} {
		b.Run(sz.name, func(b *testing.B) {
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			d := tenantSnapshot(sz.nodes, sz.identities, sz.roles, sz.perms)
			runtime.GC()
			runtime.ReadMemStats(&after)

			alloc := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			b.ReportMetric(float64(alloc)/(1<<20), "MB/tenant")
			// What an instance costs at three fleet sizes. A number per
			// tenant is easy to read past; a number per fleet is the one
			// that decides whether sharding is enough.
			b.ReportMetric(float64(alloc)*100/(1<<30), "GB/100tenants")
			b.ReportMetric(float64(alloc)*1000/(1<<30), "GB/1000tenants")
			b.ReportMetric(0, "ns/op")
			runtime.KeepAlive(d)
		})
	}
}

// tenantSnapshot builds one tenant's snapshot at a given size, with every map
// Data carries populated — a measurement that filled only Scope would report
// the cost of the cheapest part.
func tenantSnapshot(nodes, identities, roles, perms int) *Data {
	d := &Data{
		TenantID:        "00000000-0000-0000-0000-000000000000",
		TenantSlug:      "probe",
		StrictAxes:      map[string]bool{"org": true},
		RolePermissions: make(map[string]map[string]bool, roles),
		Permissions:     make(map[string]Permission, perms),
		Identities:      make(map[string]Identity, identities),
		RevokedSessions: map[string]bool{},
	}

	permKeys := make([]string, perms)
	for i := range permKeys {
		permKeys[i] = fmt.Sprintf("app:resource%04d:action", i)
		d.Permissions[permKeys[i]] = Permission{Key: permKeys[i], MinAssurance: 1}
	}
	// Effective permissions are FLATTENED, so a role holds every permission
	// it inherits. Half the catalogue per role is a deliberate middle: role
	// graphs in practice are neither one-permission leaves nor full grants.
	for r := 0; r < roles; r++ {
		role := fmt.Sprintf("role-%04d", r)
		set := make(map[string]bool, perms/2)
		for i := 0; i < perms/2; i++ {
			set[permKeys[i]] = true
		}
		d.RolePermissions[role] = set
	}

	sn := make([]ScopeNode, nodes)
	for i := range sn {
		sn[i] = ScopeNode{ID: fmt.Sprintf("%036d", i)}
		if i > 0 {
			sn[i].Parent = fmt.Sprintf("%036d", (i-1)/8)
		}
	}
	d.Scope = NewScopeIndex(sn)

	d.GrantsByIdentity = make(map[string][]Grant, identities)
	from := time.Now().Add(-time.Hour)
	for i := 0; i < identities; i++ {
		id := fmt.Sprintf("identity-%08d", i)
		d.Identities[id] = Identity{TokenEpoch: 1, AssuranceLevel: 2}
		d.GrantsByIdentity[id] = []Grant{{
			ID:        fmt.Sprintf("grant-%08d", i),
			RoleID:    fmt.Sprintf("role-%04d", i%roles),
			ValidFrom: from,
			Scopes: map[string][]ScopeConstraint{"org": {{
				NodeID: fmt.Sprintf("%036d", i%nodes), Inherit: true,
			}}},
		}}
	}
	d.InternGrantScopes()
	return d
}
