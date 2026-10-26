package convert

import (
	"sort"
	"strings"

	"github.com/gsoultan/anubis/cmd/osiris2anubis/osiris"
	authzcatalog "github.com/gsoultan/anubis/internal/authz/app/catalog"
)

// RuleMapping is one line of rules.csv: what the Osiris side asks for, and
// what it asks Anubis for instead.
type RuleMapping struct {
	Rule       string
	Permission string // "app:resource:action", or "" for a scope marker
	Marker     bool
}

// CatalogResult is the manifest, the rule table and the role names Osiris
// policies became — the latter is what the people export grants.
type CatalogResult struct {
	Manifest authzcatalog.Manifest
	Rules    []RuleMapping
	// RoleByPolicy: Osiris policy id → stored Anubis role name (app.slug).
	RoleByPolicy map[string]string
	Report       *Report
}

// Catalog converts the rule catalog. app is the Anubis application slug the
// manifest is applied to; it prefixes every key and role name.
func Catalog(app string, c osiris.Catalog, rep *Report) CatalogResult {
	out := CatalogResult{RoleByPolicy: map[string]string{}, Report: rep}

	moduleOfFeature := map[string]string{}
	moduleName := map[string]string{}
	for _, m := range c.Modules {
		moduleName[m.ID] = m.Name
	}
	for _, f := range c.Features {
		moduleOfFeature[f.ID] = moduleName[f.ModuleID]
	}
	policyByID := map[string]osiris.Policy{}
	for _, p := range c.Policies {
		policyByID[p.ID] = p
	}
	rulesOfPolicy := map[string][]string{}
	modulesOfRule := map[string]map[string]int{}
	referenced := map[string]bool{}
	for _, a := range c.Authorities {
		rulesOfPolicy[a.PolicyID] = append(rulesOfPolicy[a.PolicyID], a.RuleID)
		referenced[a.RuleID] = true
		if p, ok := policyByID[a.PolicyID]; ok {
			if mod := moduleOfFeature[p.FeatureID]; mod != "" {
				if modulesOfRule[a.RuleID] == nil {
					modulesOfRule[a.RuleID] = map[string]int{}
				}
				modulesOfRule[a.RuleID][mod]++
			}
		}
	}

	// Permissions: one per rule that is not a marker. Two rules that clean to
	// the same key are a conflict Osiris never had, because it compared whole
	// names; the second keeps its WHOLE name as the resource so nothing is
	// silently merged, and the report says so.
	keyOfRule := map[string]string{}
	taken := map[string]string{}
	rules := append([]osiris.Rule(nil), c.Rules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	for _, r := range rules {
		if IsScopeMarker(r.Name) {
			out.Rules = append(out.Rules, RuleMapping{Rule: r.Name, Marker: true})
			rep.ScopeMarkers = append(rep.ScopeMarkers, r.Name)
			continue
		}
		resource, action := RuleKey(r.Name)
		key := resource + ":" + action
		if other, dup := taken[key]; dup {
			rep.KeyConflicts = append(rep.KeyConflicts, r.Name+" and "+other+" both map to "+key)
			resource, action = clean(r.Name), "use"
			key = resource + ":" + action
		}
		taken[key] = r.Name
		keyOfRule[r.ID] = key
		if !referenced[r.ID] {
			rep.UnreferencedRules = append(rep.UnreferencedRules, r.Name)
		}
		out.Manifest.Permissions = append(out.Manifest.Permissions, authzcatalog.Permission{
			Resource: resource, Action: action,
			Description: strings.TrimSpace(r.Description),
			Module:      commonest(modulesOfRule[r.ID]),
		})
		out.Rules = append(out.Rules, RuleMapping{Rule: r.Name, Permission: app + ":" + key})
	}

	// Roles: one per policy, carrying the policy's rules. Markers in a policy
	// are not permissions; they are remembered so the people export can turn
	// them into reach.
	policies := append([]osiris.Policy(nil), c.Policies...)
	sort.Slice(policies, func(i, j int) bool { return policies[i].Name < policies[j].Name })
	slugs := map[string]bool{}
	for _, p := range policies {
		slug := Slug(p.Name)
		for n := 2; slugs[slug]; n++ {
			slug = Slug(p.Name) + "-" + itoa(n)
		}
		slugs[slug] = true
		role := authzcatalog.Role{Name: slug, Description: strings.TrimSpace(p.Description)}
		for _, rid := range rulesOfPolicy[p.ID] {
			if key, ok := keyOfRule[rid]; ok {
				role.Permissions = append(role.Permissions, key)
			} else if rep != nil {
				rep.policyHasMarker[p.ID] = true
			}
		}
		sort.Strings(role.Permissions)
		out.Manifest.Roles = append(out.Manifest.Roles, role)
		out.RoleByPolicy[p.ID] = app + "." + slug
	}
	rep.Rules, rep.Permissions, rep.Roles, rep.Groups = len(c.Rules), len(out.Manifest.Permissions), len(out.Manifest.Roles), len(c.Groups)
	return out
}

// commonest picks the module most policies file the rule under; ties go to
// the alphabetically first, so the answer is the same on every run.
func commonest(counts map[string]int) string {
	best, bestN := "", 0
	for name, n := range counts {
		if n > bestN || (n == bestN && name < best) {
			best, bestN = name, n
		}
	}
	return best
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
