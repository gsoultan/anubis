package convert

import (
	"sort"
	"strings"

	"github.com/gsoultan/anubis/cmd/osiris2anubis/osiris"
)

// PeopleResult is the two sheets of the import workbook, as rows in the
// column order internal/provisioning/domain/schema declares.
type PeopleResult struct {
	People [][]string // realm, username, email, category, external_ref, assurance_level
	Grants [][]string // realm, username, role, scope_axis, scope_ref, scope_inherit, scope_exclude, valid_until, reason
}

// People converts who exists and what they hold.
//
// Osiris gives access by GROUP MEMBERSHIP AT A PLACEMENT: the user sits in
// group G at (org, unit, workplace), and G confers policies. In Anubis that
// is a grant of each policy's role, scoped to the org AND the unit AND the
// workplace the placement names — one grant row per axis, which the import
// folds into one grant with several scopes. Grade and job type are a
// position, not a place: scoping a grant to "grade 5" would revoke access on
// promotion. They are dropped and counted.
//
// A group that confers a scope-marker policy (FULL_DATA_SCOPE and friends)
// said "sees everything" in Osiris; its roles are granted everywhere.
func People(o Options, cat CatalogResult, c osiris.Catalog, p osiris.People, rep *Report) PeopleResult {
	var out PeopleResult

	userByID := map[string]osiris.User{}
	realmOf := map[string]string{}
	usernameOf := map[string]string{}
	seen := map[string]bool{}
	users := append([]osiris.User(nil), p.Users...)
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	for _, u := range users {
		realm := o.Realms[strings.ToUpper(u.EntityType)]
		if realm == "" {
			rep.PeopleByRealm["(skipped: "+u.EntityType+")"]++
			continue
		}
		// Anubis matches lower(username); Osiris stored the case people
		// typed. Two Osiris accounts that differ only by case are one Anubis
		// identity, and the first keeps it.
		username := strings.ToLower(strings.TrimSpace(u.Username))
		if seen[realm+"/"+username] {
			rep.DuplicateUsernames = append(rep.DuplicateUsernames, realm+"/"+u.Username)
			continue
		}
		seen[realm+"/"+username] = true
		email := ""
		if strings.Contains(username, "@") {
			email = username
		}
		userByID[u.ID] = u
		realmOf[u.ID], usernameOf[u.ID] = realm, username
		rep.PeopleByRealm[realm]++
		out.People = append(out.People, []string{realm, username, email, "", u.ID, ""})
	}

	// Which roles a group confers, and whether any of them is "everything".
	rolesOfGroup := map[string][]string{}
	everywhereGroup := map[string]bool{}
	groupCode := map[string]string{}
	for _, g := range c.Groups {
		groupCode[g.ID] = g.Code
		set := map[string]bool{}
		for _, pid := range g.PolicyIDs {
			if role, ok := cat.RoleByPolicy[pid]; ok {
				set[role] = true
			}
			if rep.policyHasMarker[pid] {
				everywhereGroup[g.ID] = true
			}
		}
		for role := range set {
			rolesOfGroup[g.ID] = append(rolesOfGroup[g.ID], role)
		}
		sort.Strings(rolesOfGroup[g.ID])
	}

	rowSeen := map[string]bool{}
	add := func(row []string) {
		k := strings.Join(row, "\x00")
		if rowSeen[k] {
			return
		}
		rowSeen[k] = true
		out.Grants = append(out.Grants, row)
	}
	placements := append([]osiris.Placement(nil), p.Placements...)
	sort.Slice(placements, func(i, j int) bool { return placements[i].ID < placements[j].ID })
	for _, pl := range placements {
		realm, ok := realmOf[pl.UserID]
		if !ok {
			continue
		}
		if len(pl.GroupIDs) == 0 {
			rep.NoGroup++
			continue
		}
		if pl.GradeID != "" {
			rep.DroppedGrade++
		}
		if pl.JobTypeID != "" {
			rep.DroppedJobType++
		}
		var places [][2]string
		for _, ax := range [][2]string{{o.AxisOrg, pl.OrganizationID}, {o.AxisUnit, pl.UnitID}, {o.AxisWorkplace, pl.WorkplaceID}} {
			if ax[0] != "" && ax[1] != "" {
				places = append(places, ax)
			}
		}
		unplaced := len(places) == 0
		if unplaced {
			rep.Unplaced++
			if !o.UnplacedEverywhere {
				continue
			}
		}
		username := usernameOf[pl.UserID]
		for _, gid := range pl.GroupIDs {
			reason := "osiris group " + groupCode[gid]
			for _, role := range rolesOfGroup[gid] {
				if unplaced || everywhereGroup[gid] {
					add([]string{realm, username, role, "", "", "", "", "", reason})
					continue
				}
				for _, ax := range places {
					add([]string{realm, username, role, ax[0], ax[1], "true", "false", "", reason})
				}
			}
		}
	}
	rep.GrantRows = len(out.Grants)
	rep.UnplacedIncluded = o.UnplacedEverywhere
	return out
}
