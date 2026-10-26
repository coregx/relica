package core

import (
	"errors"
	"fmt"

	"github.com/coregx/relica/internal/dialects"
)

// MySQLUpsertSyntax selects how MySQL upserts refer to the proposed row (ADR-019).
type MySQLUpsertSyntax int

const (
	// MySQLUpsertValues renders "col = VALUES(col)": accepted by MySQL 5.7, 8.x, 9.x
	// and MariaDB. MySQL deprecated VALUES() here in 8.0.20 (warning 1287) but still
	// accepts it.
	MySQLUpsertValues MySQLUpsertSyntax = iota
	// MySQLUpsertRowAlias renders "VALUES (...) AS excluded ... col = excluded.col":
	// MySQL 8.0.19 and later only; MariaDB rejects it.
	MySQLUpsertRowAlias
)

// WithMySQLUpsertSyntax selects the MySQL upsert syntax. It is a static choice:
// SQL is fixed in Build() and returned by ToSQL() without a connection, so the
// server version is never probed. Other dialects ignore it.
func WithMySQLUpsertSyntax(s MySQLUpsertSyntax) Option {
	return func(db *DB) {
		db.mysqlUpsertSyntax = s
	}
}

type upsertAction int

const (
	upsertNoAction upsertAction = iota
	upsertDoUpdate
	upsertDoUpdateAll
	upsertDoNothing
)

const upsertOneAction = "exactly one of DoUpdate(cols...), DoUpdateAll(), DoNothing()"

// conflictClause collects OnConflict and the conflict action of an upsert
// (UpsertQuery, BatchInsertQuery). Misuse is recorded and reported at Build.
type conflictClause struct {
	target    []string
	targetSet bool
	action    upsertAction
	update    []string
	err       error
}

func (c *conflictClause) onConflict(cols []string) {
	switch {
	case c.err != nil:
	case len(cols) == 0:
		c.err = errors.New("relica: OnConflict requires at least one column")
	case c.targetSet:
		c.err = errors.New("relica: OnConflict called more than once")
	default:
		c.target = cols
		c.targetSet = true
	}
}

func (c *conflictClause) setAction(a upsertAction, cols []string) {
	switch {
	case c.err != nil:
	case c.action != upsertNoAction:
		c.err = errors.New("relica: an upsert takes " + upsertOneAction + "; a second action was given")
	case a == upsertDoUpdate && len(cols) == 0:
		c.err = errors.New("relica: DoUpdate requires at least one column; use DoUpdateAll() to update every inserted column")
	default:
		c.action = a
		c.update = cols
	}
}

// used reports whether the caller asked for any conflict handling.
func (c *conflictClause) used() bool {
	return c.err != nil || c.targetSet || c.action != upsertNoAction
}

// render validates the clause against the inserted columns (in INSERT order)
// and returns the dialect SQL to append after VALUES.
func (c *conflictClause) render(db *DB, inserted []string) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	if c.action == upsertNoAction {
		return "", errors.New("relica: an upsert takes " + upsertOneAction)
	}
	if len(inserted) == 0 {
		return "", errors.New("relica: an upsert needs at least one inserted column")
	}

	_, isMySQL := db.dialect.(*dialects.MySQLDialect)
	if c.action != upsertDoNothing && !c.targetSet && !isMySQL {
		// ON CONFLICT DO UPDATE needs a conflict target on PostgreSQL and SQLite.
		return "", errors.New("relica: DoUpdate/DoUpdateAll needs OnConflict(cols...) on PostgreSQL and SQLite")
	}

	update, err := c.updateColumns(inserted)
	if err != nil {
		return "", err
	}

	u := dialects.Upsert{
		Target:    quoteAll(db.dialect, c.target),
		Update:    quoteAll(db.dialect, update),
		DoNothing: c.action == upsertDoNothing,
		RowAlias:  db.mysqlUpsertSyntax == MySQLUpsertRowAlias,
	}
	if u.DoNothing {
		noOp := inserted[0]
		if c.targetSet {
			noOp = c.target[0]
		}
		u.NoOp = db.dialect.QuoteIdentifier(noOp)
	}
	return db.dialect.UpsertSQL(u), nil
}

// updateColumns returns the columns DoUpdate/DoUpdateAll set; nil for DoNothing.
func (c *conflictClause) updateColumns(inserted []string) ([]string, error) {
	switch c.action {
	case upsertDoUpdate:
		insertedSet := make(map[string]bool, len(inserted))
		for _, col := range inserted {
			insertedSet[col] = true
		}
		seen := make(map[string]bool, len(c.update))
		for _, col := range c.update {
			if !insertedSet[col] {
				// Its "proposed row" value would be the column default or NULL.
				return nil, fmt.Errorf("relica: DoUpdate column %q is not among the inserted columns", col)
			}
			if seen[col] {
				return nil, fmt.Errorf("relica: DoUpdate column %q is listed twice", col)
			}
			seen[col] = true
		}
		return c.update, nil
	case upsertDoUpdateAll:
		// The target stays out of SET on every dialect: on MySQL a row matched
		// through another unique key must not get its target columns overwritten.
		update := filterKeys(inserted, c.target)
		if len(update) == 0 {
			return nil, errors.New("relica: DoUpdateAll has no column to update: every inserted column is in the conflict target")
		}
		return update, nil
	default:
		return nil, nil
	}
}

func quoteAll(d dialects.Dialect, cols []string) []string {
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = d.QuoteIdentifier(c)
	}
	return q
}
