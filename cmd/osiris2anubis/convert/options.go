package convert

// Options is what the operator decides and the conversion must not guess.
type Options struct {
	// App is the Anubis application slug the manifest is applied to. It
	// prefixes every permission key and role name.
	App string
	// Realms maps an Osiris entity_type (EMPLOYEE, APPLICANT, REGISTRANT,
	// DEVICE) to the Anubis realm code those people go into. A type with no
	// mapping is skipped and counted.
	Realms map[string]string
	// AxisOrg, AxisUnit, AxisWorkplace are the Anubis structure codes that
	// Osiris's organisation, unit and workplace were synced into. The
	// Osiris ids are used as scope references, which is what a structure
	// synced from Osiris's own tables carries as external_ref.
	AxisOrg, AxisUnit, AxisWorkplace string
	// UnplacedEverywhere grants a person whose placement names no place at
	// all the role EVERYWHERE — Osiris's actual behaviour, and Anubis's
	// widest grant. Off by default: those people are skipped and counted,
	// because keeping full access is the one outcome a migration meant to
	// narrow access must not produce by accident.
	UnplacedEverywhere bool
}

// DefaultOptions is the shape of the Impack installation.
func DefaultOptions() Options {
	return Options{
		App: "osiris",
		Realms: map[string]string{
			"EMPLOYEE": "internal", "DEVICE": "internal",
			"APPLICANT": "public", "REGISTRANT": "public",
		},
		AxisOrg: "org", AxisUnit: "unit", AxisWorkplace: "workplace",
	}
}
