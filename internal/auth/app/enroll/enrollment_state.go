package enroll

// enrollmentState is the payload sealed inside the enrolment token: the
// pending TOTP secret, bound to the identity that requested it.
type enrollmentState struct {
	IdentityID string `json:"identity_id"`
	TenantID   string `json:"tenant_id"`
	Secret     []byte `json:"secret"`
	Issuer     string `json:"issuer"`
	Account    string `json:"account"`
}
