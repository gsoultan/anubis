package postgres

import "github.com/gsoultan/anubis/internal/repository"

// Compile-time proof that Store satisfies the full composite. If a repository
// interface gains a method, this line breaks before anything ships.
var _ repository.Store = (*Store)(nil)
