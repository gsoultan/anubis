package ratelimit

// KeyLimit pairs one axis key (ip:..., acct:..., tenant:...) with its class.
type KeyLimit struct {
	Key   string
	Limit Limit
}
