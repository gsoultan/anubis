package authzcatalog

import (
	"strconv"
	"strings"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// Risk levels a permission may declare; the column's CHECK constraint says
// the same. Catching it here names the row and the value instead of failing
// as a constraint violation halfway through a transaction.
var risks = map[string]bool{"normal": true, "sensitive": true, "critical": true}

// Validate refuses a document that cannot be applied, BEFORE anything is
// written. Every check here is one an operator can act on from the message
// alone — a CSV is usually somebody's export, and "invalid argument" against
// a file of four hundred rows is not a fault report.
func (d *Document) Validate() error {
	seen := make(map[string]bool, len(d.Permissions))
	for i, p := range d.Permissions {
		where := "row " + strconv.Itoa(i+1)
		if p.Resource == "" || p.Action == "" {
			return apperr.ErrInvalidArgument.
				With("manifest", "permission missing resource/action").With("at", where)
		}
		key := p.Resource + ":" + p.Action
		// The duplicate is the reason to check: a spreadsheet merged from two
		// teams names the same permission twice, the second row silently wins,
		// and the description everyone reviewed is not the one that landed.
		if seen[key] {
			return apperr.ErrInvalidArgument.
				With("permission", key).With("at", where).
				With("conflict", "declared twice in this document")
		}
		seen[key] = true
		if p.Risk != "" && !risks[strings.ToLower(p.Risk)] {
			return apperr.ErrInvalidArgument.
				With("permission", key).With("risk", p.Risk).
				With("expected", "normal, sensitive or critical")
		}
		if p.MinAssurance < 0 || p.MinAssurance > 3 {
			return apperr.ErrInvalidArgument.
				With("permission", key).
				With("min_assurance", strconv.Itoa(p.MinAssurance)).
				With("expected", "0 (unset), 1, 2 or 3")
		}
	}

	roles := make(map[string]bool, len(d.Roles))
	for i, r := range d.Roles {
		where := "row " + strconv.Itoa(i+1)
		if strings.TrimSpace(r.Name) == "" {
			return apperr.ErrInvalidArgument.
				With("manifest", "role missing name").With("at", where)
		}
		if roles[r.Name] {
			return apperr.ErrInvalidArgument.
				With("role", r.Name).With("at", where).
				With("conflict", "declared twice in this document")
		}
		roles[r.Name] = true
	}

	prio := map[int]string{}
	for _, r := range d.Routes {
		switch r.Effect {
		case "public", "require_auth", "require_permission", "deny":
		default:
			return apperr.ErrInvalidArgument.With("route", r.PathPattern).With("effect", r.Effect)
		}
		if prev, dup := prio[r.Priority]; dup {
			return apperr.ErrInvalidArgument.With("route", r.PathPattern).
				With("conflict", prev).With("priority", strconv.Itoa(r.Priority))
		}
		prio[r.Priority] = r.PathPattern
	}
	return nil
}
