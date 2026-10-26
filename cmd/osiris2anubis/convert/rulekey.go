// Package convert turns Osiris's catalog and people into what Anubis
// imports: a manifest (permissions and roles), a rule→permission table for
// the Osiris side's CheckRule adapter, and the people-and-access workbook.
//
// Every rule here is deterministic and written down, because the Osiris
// code will keep asking by rule NAME for a long time and the mapping has to
// come out the same on both sides every time it is run.
package convert

import (
	"regexp"
	"strings"
)

// Rule names are VERB_NOUN[_QUALIFIER]: LIST_TRAINING, ADD_NEW_TRAINING,
// VIEW_EVALUATION_FORM. The verb becomes the action, the rest the resource:
//
//	LIST_TRAINING        → training:list
//	ADD_NEW_TRAINING     → new_training:add
//	VIEW_EVALUATION_FORM → evaluation_form:view
//	SHARE                → share:use        (a single word has no noun)
//
// The split is on the FIRST underscore only, so the resource keeps whatever
// Osiris put after the verb, word for word. Prettier keys would need a
// dictionary, and a dictionary is a second thing to keep in sync.
//
// A few rules are not permissions at all: FULL_DATA_SCOPE and the
// *_FULL_SCOPE family mark "may see everything", which in Anubis is a grant
// with no scope, not a permission. IsScopeMarker names them so the converter
// turns them into reach instead of keys.

var keyPart = regexp.MustCompile(`[^a-z0-9_]+`)

// RuleKey maps an Osiris rule name to resource and action.
func RuleKey(rule string) (resource, action string) {
	name := strings.ToUpper(strings.TrimSpace(rule))
	verb, rest, found := strings.Cut(name, "_")
	if !found || rest == "" {
		return clean(name), "use"
	}
	return clean(rest), clean(verb)
}

// IsScopeMarker reports whether a rule is a reach marker rather than an
// action: FULL_DATA_SCOPE, or anything ending in _FULL_SCOPE.
func IsScopeMarker(rule string) bool {
	name := strings.ToUpper(strings.TrimSpace(rule))
	return name == "FULL_DATA_SCOPE" || strings.HasSuffix(name, "_FULL_SCOPE")
}

// clean lowercases and keeps [a-z0-9_], collapsing anything else to one
// underscore, so a key is always something Anubis will store.
func clean(s string) string {
	s = keyPart.ReplaceAllString(strings.ToLower(s), "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "unnamed"
	}
	if s[0] < 'a' || s[0] > 'z' {
		s = "r_" + s
	}
	return s
}

// Slug makes a role or membership name out of a free-text Osiris name:
// lowercase, words joined by hyphens, nothing else.
func Slug(s string) string {
	s = keyPart.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "unnamed"
	}
	return s
}
