package authzdomain

import (
	"strings"
	"time"
)

// StepUpDecision is a machine-readable "not with THIS authentication" — the
// caller knows exactly what to do next instead of guessing.
type StepUpDecision struct {
	RequiredAMR []string
	MaxAuthAge  time.Duration
	CurrentAMR  []string
	AuthAge     time.Duration
}

// EvaluateStepUp returns nil when the presented authentication satisfies the
// permission, else the decision detail. A zero AuthTime with a recency
// requirement fails closed — unknown auth age is old auth age.
func EvaluateStepUp(req PermissionRequirements, amr []string, authTime time.Time, now time.Time) *StepUpDecision {
	missing := false
	have := make(map[string]bool, len(amr))
	for _, m := range amr {
		have[strings.TrimSpace(m)] = true
	}
	for _, m := range req.RequiresAMR {
		if !have[m] {
			missing = true
			break
		}
	}
	tooOld := false
	var age time.Duration
	if req.MaxAuthAge > 0 {
		if authTime.IsZero() {
			tooOld = true
		} else {
			age = now.Sub(authTime)
			tooOld = age > req.MaxAuthAge
		}
	}
	if !missing && !tooOld {
		return nil
	}
	return &StepUpDecision{
		RequiredAMR: req.RequiresAMR,
		MaxAuthAge:  req.MaxAuthAge,
		CurrentAMR:  amr,
		AuthAge:     age,
	}
}
