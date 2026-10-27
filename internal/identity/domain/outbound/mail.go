package outbound

// Mail is one message Anubis asks to have delivered to a person: a reset
// link, a verification link. Plain text only — these messages carry a
// secret, and a secret inside markup is a secret inside whatever that
// markup gets pasted into.
type Mail struct {
	To      string
	Subject string
	Text    string
}
