package authzdomain

import (
	"testing"
	"time"
)

func TestEvaluateStepUp(t *testing.T) {
	now := time.Now()
	req := PermissionRequirements{RequiresAMR: []string{"otp"}, MaxAuthAge: 5 * time.Minute}

	if d := EvaluateStepUp(req, []string{"pwd", "otp"}, now.Add(-time.Minute), now); d != nil {
		t.Fatalf("fresh otp auth must pass, got %+v", d)
	}
	if d := EvaluateStepUp(req, []string{"pwd"}, now.Add(-time.Minute), now); d == nil {
		t.Fatal("missing otp must demand step-up")
	}
	if d := EvaluateStepUp(req, []string{"pwd", "otp"}, now.Add(-41*time.Minute), now); d == nil {
		t.Fatal("stale auth must demand step-up")
	} else if d.AuthAge < 40*time.Minute {
		t.Errorf("auth age misreported: %v", d.AuthAge)
	}
	// Unknown auth time with a recency requirement fails CLOSED.
	if d := EvaluateStepUp(req, []string{"pwd", "otp"}, time.Time{}, now); d == nil {
		t.Fatal("zero auth_time with max_auth_age must fail closed")
	}
	// No requirements: always satisfied.
	if d := EvaluateStepUp(PermissionRequirements{}, nil, time.Time{}, now); d != nil {
		t.Fatalf("no requirements must pass, got %+v", d)
	}
}
