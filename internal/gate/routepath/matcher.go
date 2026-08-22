package routepath

import (
	"strings"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
)

// Match finds the highest-priority route whose pattern, host and method
// accept the request. Patterns are segment-wise:
//
//	/invoices/{id}   one segment captured as id
//	/reports/*       one arbitrary segment
//	/static/**       any suffix (also written as a trailing /*)
//
// Routes arrive pre-sorted by priority; explicit integer priority beats
// implicit "most specific wins" — authorization surprises are outages.
func Match(routes []snapshot.Route, host, method, normPath string) (*snapshot.Route, map[string]string) {
	for i := range routes {
		r := &routes[i]
		if r.HostPattern != "" && !hostMatch(r.HostPattern, host) {
			continue
		}
		if !methodMatch(r.Methods, method) {
			continue
		}
		if params, ok := pathMatch(r.PathPattern, normPath); ok {
			return r, params
		}
	}
	return nil, nil
}

func methodMatch(methods []string, m string) bool {
	for _, allow := range methods {
		if allow == "*" || strings.EqualFold(allow, m) {
			return true
		}
	}
	return len(methods) == 0
}

func hostMatch(pattern, host string) bool {
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return strings.EqualFold(pattern, host)
}

func pathMatch(pattern, path string) (map[string]string, bool) {
	ps := splitSegs(pattern)
	xs := splitSegs(path)
	var params map[string]string

	for i, seg := range ps {
		if seg == "**" {
			// terminal-only: swallows ONE OR MORE remaining segments.
			return params, i == len(ps)-1 && len(xs) > i
		}
		if i >= len(xs) {
			return nil, false
		}
		switch {
		case seg == "*":
			// exactly one arbitrary segment
		case len(seg) > 1 && seg[0] == '{' && seg[len(seg)-1] == '}':
			if params == nil {
				params = map[string]string{}
			}
			params[seg[1:len(seg)-1]] = xs[i]
		default:
			if seg != xs[i] {
				return nil, false
			}
		}
	}
	return params, len(xs) == len(ps)
}

func splitSegs(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
