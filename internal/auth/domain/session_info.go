package authdomain

import "time"

type SessionInfo struct {
	ID         string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	AMR        []string
	IP         string
	UserAgent  string
}
