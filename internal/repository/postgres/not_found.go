package postgres

import "github.com/gsoultan/anubis/internal/domain"

func notFoundErr() error { return domain.ErrNotFound }
