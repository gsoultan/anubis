package routepath

import "github.com/gsoultan/anubis/internal/gate/snapshot"

// benchRoutes mirrors a realistic policy list: a handful of public paths, a
// parameterised protected one, and a catch-all — matched in priority order.
func benchRoutes() []snapshot.Route {
	return []snapshot.Route{
		{Priority: 10, Effect: "public", PathPattern: "/public/**", Methods: []string{"*"}},
		{Priority: 15, Effect: "public", PathPattern: "/healthz", Methods: []string{"GET"}},
		{Priority: 20, Effect: "require_permission", PathPattern: "/invoices/{id}",
			Methods: []string{"GET"}, PermissionKey: "billing:invoice:read"},
		{Priority: 30, Effect: "require_permission", PathPattern: "/reports/*",
			Methods: []string{"GET"}, PermissionKey: "billing:report:read"},
		{Priority: 90, Effect: "require_auth", PathPattern: "/**", Methods: []string{"*"}},
	}
}
