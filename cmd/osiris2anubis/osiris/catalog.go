package osiris

// Catalog is everything about WHAT can be done and WHO is bundled how.
type Catalog struct {
	Rules       []Rule
	Policies    []Policy
	Authorities []Authority
	Features    []Feature
	Modules     []Module
	Apps        []App
	Groups      []Group
}
