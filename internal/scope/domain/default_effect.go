package scopedomain

import "fmt"

// The two effects an axis can have when a grant does not name it.
//
// This is the single most consequential setting on an axis, and the names
// undersell it: "unconstrained" means a scope naming a node the subject holds
// no grant on is IGNORED, so the axis narrows nothing. "deny" means a grant
// that does not mention the axis does not satisfy it.
const (
	// EffectDeny is strict and fails closed.
	EffectDeny = "deny"

	// EffectUnconstrained ignores the axis unless a grant names it.
	EffectUnconstrained = "unconstrained"
)

// DefaultEffectForNewAxis is what an axis gets when its creator does not say.
//
// DENY, deliberately. An axis exists to narrow access; one created with no
// effect stated has not been reasoned about yet, and the safe reading of
// silence is the one that refuses rather than the one that ignores. Somebody
// who wants the permissive behaviour can ask for it by name, and the asking is
// the point — an axis nobody decided about should not quietly widen every grant
// that omits it.
//
// This changes NEW axes only. Existing ones keep whatever they have; flipping
// those is what StrictDryRun exists for, because it tightens every grant that
// omits the axis and that wants measuring first.
const DefaultEffectForNewAxis = EffectDeny

// NormaliseDefaultEffect fills in the default and rejects anything else.
//
// Validated here rather than left to the database's CHECK constraint, which
// answers a typo with a raw constraint violation naming neither the field nor
// the two values that would have worked.
func NormaliseDefaultEffect(effect string) (string, error) {
	switch effect {
	case "":
		return DefaultEffectForNewAxis, nil
	case EffectDeny, EffectUnconstrained:
		return effect, nil
	default:
		return "", fmt.Errorf("default_effect %q is neither %q nor %q", effect, EffectDeny, EffectUnconstrained)
	}
}
