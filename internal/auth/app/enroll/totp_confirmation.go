package enroll

// TOTPConfirmation is the second phase. Recovery codes are returned exactly
// once; Anubis keeps only their hashes.
type TOTPConfirmation struct {
	CredentialID  string
	RecoveryCodes []string
}
