package tenancyapp

import authzport "github.com/gsoultan/anubis/internal/authz/port"

// authzRepo is the slice of the authorization context this package needs:
// every admin operation is gated by authorize() itself, so delegated admins
// manage only the pages their grants scope them to.
type authzRepo = authzport.AuthzRepository
