package repository

import "time"

type ConsentRecord struct {
	ID            string
	IdentityID    string
	Purpose       string
	PolicyVersion string
	GrantedAt     time.Time
	WithdrawnAt   *time.Time
	ExpiresAt     *time.Time
}
