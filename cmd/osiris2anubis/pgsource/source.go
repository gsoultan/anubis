// Package pgsource reads the Osiris tables out of its PostgreSQL database.
// Read-only: every statement is a SELECT, and the connection is opened with
// a single read transaction so the export sees one consistent moment.
package pgsource

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"

	"github.com/gsoultan/anubis/cmd/osiris2anubis/osiris"
)

type Source struct {
	db *sql.DB
}

func Open(ctx context.Context, dsn string) (*Source, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("osiris: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("osiris: connect: %w", err)
	}
	return &Source{db: db}, nil
}

func (s *Source) Close() error { return s.db.Close() }

// Catalog reads rules, policies, their bundling, and the feature tree.
func (s *Source) Catalog(ctx context.Context) (osiris.Catalog, error) {
	var c osiris.Catalog
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return c, err
	}
	defer tx.Rollback()

	err = each(ctx, tx, qRules, func(sc scanner) error {
		var r osiris.Rule
		if err := sc.Scan(&r.ID, &r.Name, &r.Description); err != nil {
			return err
		}
		c.Rules = append(c.Rules, r)
		return nil
	})
	if err != nil {
		return c, err
	}
	err = each(ctx, tx, qPolicies, func(sc scanner) error {
		var p osiris.Policy
		if err := sc.Scan(&p.ID, &p.Name, &p.Description, &p.FeatureID); err != nil {
			return err
		}
		c.Policies = append(c.Policies, p)
		return nil
	})
	if err != nil {
		return c, err
	}
	err = each(ctx, tx, qAuthorities, func(sc scanner) error {
		var a osiris.Authority
		if err := sc.Scan(&a.PolicyID, &a.RuleID); err != nil {
			return err
		}
		c.Authorities = append(c.Authorities, a)
		return nil
	})
	if err != nil {
		return c, err
	}
	err = each(ctx, tx, qFeatures, func(sc scanner) error {
		var f osiris.Feature
		if err := sc.Scan(&f.ID, &f.Name, &f.ModuleID); err != nil {
			return err
		}
		c.Features = append(c.Features, f)
		return nil
	})
	if err != nil {
		return c, err
	}
	err = each(ctx, tx, qModules, func(sc scanner) error {
		var m osiris.Module
		if err := sc.Scan(&m.ID, &m.Name, &m.AppID); err != nil {
			return err
		}
		c.Modules = append(c.Modules, m)
		return nil
	})
	if err != nil {
		return c, err
	}
	err = each(ctx, tx, qApps, func(sc scanner) error {
		var a osiris.App
		if err := sc.Scan(&a.ID, &a.Name); err != nil {
			return err
		}
		c.Apps = append(c.Apps, a)
		return nil
	})
	if err != nil {
		return c, err
	}
	groups := map[string]*osiris.Group{}
	err = each(ctx, tx, qGroups, func(sc scanner) error {
		var g osiris.Group
		if err := sc.Scan(&g.ID, &g.Code, &g.Name, &g.Description); err != nil {
			return err
		}
		c.Groups = append(c.Groups, g)
		groups[g.ID] = &c.Groups[len(c.Groups)-1]
		return nil
	})
	if err != nil {
		return c, err
	}
	err = each(ctx, tx, qGroupPolicies, func(sc scanner) error {
		var groupID, policyID string
		if err := sc.Scan(&groupID, &policyID); err != nil {
			return err
		}
		if g, ok := groups[groupID]; ok {
			g.PolicyIDs = append(g.PolicyIDs, policyID)
		}
		return nil
	})
	return c, err
}

// People reads users, placements and group memberships.
func (s *Source) People(ctx context.Context) (osiris.People, error) {
	var p osiris.People
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return p, err
	}
	defer tx.Rollback()

	err = each(ctx, tx, qUsers, func(sc scanner) error {
		var u osiris.User
		if err := sc.Scan(&u.ID, &u.Username, &u.EntityType, &u.EntityID); err != nil {
			return err
		}
		p.Users = append(p.Users, u)
		return nil
	})
	if err != nil {
		return p, err
	}
	placements := map[string]*osiris.Placement{}
	err = each(ctx, tx, qPlacements, func(sc scanner) error {
		var pl osiris.Placement
		if err := sc.Scan(&pl.ID, &pl.UserID, &pl.OrganizationID, &pl.UnitID, &pl.WorkplaceID, &pl.GradeID, &pl.JobTypeID); err != nil {
			return err
		}
		p.Placements = append(p.Placements, pl)
		placements[pl.ID] = &p.Placements[len(p.Placements)-1]
		return nil
	})
	if err != nil {
		return p, err
	}
	err = each(ctx, tx, qMembers, func(sc scanner) error {
		var scopeID, groupID string
		if err := sc.Scan(&scopeID, &groupID); err != nil {
			return err
		}
		if pl, ok := placements[scopeID]; ok {
			pl.GroupIDs = append(pl.GroupIDs, groupID)
		}
		return nil
	})
	return p, err
}

type scanner interface{ Scan(dest ...any) error }

func each(ctx context.Context, tx *sql.Tx, query string, fn func(scanner) error) error {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("osiris: %s: %w", firstWords(query), err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func firstWords(q string) string {
	if len(q) > 48 {
		return q[:48] + "…"
	}
	return q
}
