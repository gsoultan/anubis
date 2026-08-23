package enroll

// TOTPEnrollment is the first phase: what the user needs to add the account
// to an authenticator, plus the handle that carries the pending secret.
type TOTPEnrollment struct {
	ProvisioningURI string
	Secret          string
	EnrollmentToken string
	ExpiresIn       int
}
