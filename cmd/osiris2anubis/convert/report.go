package convert

import (
	"fmt"
	"sort"
	"strings"
)

// Report is what the operator reads before applying anything: counts, and
// every place the conversion had to decide something Osiris left implicit.
type Report struct {
	Rules, Permissions, Roles, Groups int
	ScopeMarkers                      []string
	KeyConflicts                      []string
	UnreferencedRules                 []string

	PeopleByRealm      map[string]int
	DuplicateUsernames []string
	GrantRows          int
	Unplaced           int // people whose placement names no org, unit or workplace
	UnplacedIncluded   bool
	DroppedGrade       int
	DroppedJobType     int
	NoGroup            int // placements with no group membership at all

	policyHasMarker map[string]bool
}

func NewReport() *Report {
	return &Report{PeopleByRealm: map[string]int{}, policyHasMarker: map[string]bool{}}
}

func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Catalog\n  rules %d → permissions %d (scope markers %d), policies → roles %d, groups %d\n",
		r.Rules, r.Permissions, len(r.ScopeMarkers), r.Roles, r.Groups)
	list(&b, "  scope markers (become 'everywhere' reach, not permissions)", r.ScopeMarkers)
	list(&b, "  KEY CONFLICTS (second rule kept under its whole name)", r.KeyConflicts)
	list(&b, "  rules no policy confers (exported, grantable by nobody)", r.UnreferencedRules)

	realms := make([]string, 0, len(r.PeopleByRealm))
	for k := range r.PeopleByRealm {
		realms = append(realms, k)
	}
	sort.Strings(realms)
	b.WriteString("People\n")
	for _, k := range realms {
		fmt.Fprintf(&b, "  realm %s: %d\n", k, r.PeopleByRealm[k])
	}
	list(&b, "  usernames that collide once lowercased (first kept)", r.DuplicateUsernames)
	fmt.Fprintf(&b, "Access\n  grant rows %d\n", r.GrantRows)
	if r.Unplaced > 0 {
		if r.UnplacedIncluded {
			fmt.Fprintf(&b, "  UNPLACED people granted EVERYWHERE: %d — this is Osiris's behaviour and the widest grant Anubis has; review before applying\n", r.Unplaced)
		} else {
			fmt.Fprintf(&b, "  unplaced people (no org/unit/workplace) SKIPPED: %d — rerun with -unplaced=everywhere to carry them\n", r.Unplaced)
		}
	}
	fmt.Fprintf(&b, "  placements with no group: %d\n  grade dropped from %d placements, job type from %d (a position is not a place)\n",
		r.NoGroup, r.DroppedGrade, r.DroppedJobType)
	return b.String()
}

func list(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s: %d\n", title, len(items))
	limit := len(items)
	if limit > 25 {
		limit = 25
	}
	for _, it := range items[:limit] {
		fmt.Fprintf(b, "    %s\n", it)
	}
	if limit < len(items) {
		fmt.Fprintf(b, "    … and %d more\n", len(items)-limit)
	}
}
