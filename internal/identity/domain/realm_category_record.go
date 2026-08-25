package identitydomain

type RealmCategoryRecord struct {
	ID          string
	RealmID     string
	Code        string
	DisplayName string
	SortOrder   int
	// IdentityCount is counted in the database. The console used to count
	// the rows it had already fetched — capped well below a real directory,
	// so the figure it showed was simply wrong.
	IdentityCount int64
}
