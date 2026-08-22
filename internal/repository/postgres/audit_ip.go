package postgres

// auditIP tolerates either representation sqlc infers for host(ip)::text.
func auditIP(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case *string:
		if t != nil {
			return *t
		}
	}
	return ""
}
