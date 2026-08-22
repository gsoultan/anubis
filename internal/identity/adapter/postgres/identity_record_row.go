package identitypg

import (
	"time"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

func identityRecordFromRow(id, username, email, realmCode, realmKind, status,
	category, externalRef string, assurance, epoch int, createdAt time.Time,
	lastLogin, disabledAt, anonymizedAt *time.Time) identitydomain.IdentityRecord {
	return identitydomain.IdentityRecord{
		ID: id, Username: username, Email: email,
		RealmCode: realmCode, RealmKind: realmKind, Status: status,
		Category: category, ExternalRef: externalRef,
		AssuranceLevel: assurance, TokenEpoch: epoch, CreatedAt: createdAt,
		LastLoginAt: lastLogin, DisabledAt: disabledAt, AnonymizedAt: anonymizedAt,
	}
}
