package core

import (
	"strings"
	"testing"
)

// ADR-019: an upsert takes exactly one explicit conflict action, invalid
// upserts fail at build time (nothing is sent), and every dialect updates from
// the proposed row (PG/SQLite EXCLUDED, MySQL VALUES() or the row alias).

func upsertValues() map[string]any {
	return map[string]any{"id": 1, "name": "Alice", "email": "a@example.com"}
}

// buildErrContains asserts that q carries a build error containing want and no SQL.
func buildErrContains(t *testing.T, q *Query, want string) {
	t.Helper()
	if q.prepErr == nil {
		t.Fatalf("expected build error containing %q, got SQL %q", want, q.sql)
	}
	if !strings.Contains(q.prepErr.Error(), want) {
		t.Fatalf("build error %q does not contain %q", q.prepErr.Error(), want)
	}
	if q.sql != "" {
		t.Fatalf("a failed build must carry no SQL, got %q", q.sql)
	}
}

func buildOK(t *testing.T, q *Query, wantSQL string) {
	t.Helper()
	if q.prepErr != nil {
		t.Fatalf("unexpected build error: %v", q.prepErr)
	}
	if q.sql != wantSQL {
		t.Fatalf("SQL mismatch\n got: %s\nwant: %s", q.sql, wantSQL)
	}
}

var allDialects = []string{"postgres", "mysql", "sqlite"}

func TestUpsert_NoAction_IsBuildError(t *testing.T) {
	for _, d := range allDialects {
		t.Run(d, func(t *testing.T) {
			qb := &QueryBuilder{db: mockDB(d)}
			buildErrContains(t, qb.Upsert("users", upsertValues()).Build(), "exactly one of DoUpdate")
			buildErrContains(t, qb.Upsert("users", upsertValues()).OnConflict("id").Build(), "exactly one of DoUpdate")
		})
	}
}

func TestUpsert_TwoActions_IsBuildError(t *testing.T) {
	for _, d := range allDialects {
		t.Run(d, func(t *testing.T) {
			qb := &QueryBuilder{db: mockDB(d)}
			cases := map[string]*UpsertQuery{
				"DoUpdate+DoNothing":    qb.Upsert("users", upsertValues()).OnConflict("id").DoUpdate("name").DoNothing(),
				"DoNothing+DoUpdateAll": qb.Upsert("users", upsertValues()).OnConflict("id").DoNothing().DoUpdateAll(),
				"DoUpdate twice":        qb.Upsert("users", upsertValues()).OnConflict("id").DoUpdate("name").DoUpdate("email"),
				"DoNothing twice":       qb.Upsert("users", upsertValues()).OnConflict("id").DoNothing().DoNothing(),
			}
			for name, uq := range cases {
				t.Run(name, func(t *testing.T) {
					buildErrContains(t, uq.Build(), "exactly one of DoUpdate")
				})
			}
		})
	}
}

func TestUpsert_InvalidArguments_AreBuildErrors(t *testing.T) {
	for _, d := range allDialects {
		t.Run(d, func(t *testing.T) {
			qb := &QueryBuilder{db: mockDB(d)}
			buildErrContains(t, qb.Upsert("users", upsertValues()).OnConflict().DoNothing().Build(),
				"OnConflict requires at least one column")
			buildErrContains(t, qb.Upsert("users", upsertValues()).OnConflict("id").OnConflict("email").DoNothing().Build(),
				"OnConflict called more than once")
			buildErrContains(t, qb.Upsert("users", upsertValues()).OnConflict("id").DoUpdate().Build(),
				"DoUpdate requires at least one column")
			buildErrContains(t, qb.Upsert("users", upsertValues()).OnConflict("id").DoUpdate("status").Build(),
				`DoUpdate column "status" is not among the inserted columns`)
			buildErrContains(t, qb.Upsert("users", upsertValues()).OnConflict("id").DoUpdate("name", "name").Build(),
				`DoUpdate column "name" is listed twice`)
			buildErrContains(t, qb.Upsert("users", map[string]any{"id": 1}).OnConflict("id").DoUpdateAll().Build(),
				"DoUpdateAll has no column to update")
			buildErrContains(t, qb.Upsert("users", map[string]any{}).OnConflict("id").DoNothing().Build(),
				"Upsert requires a non-empty values map")
		})
	}
}

func TestUpsert_DoUpdateWithoutTarget(t *testing.T) {
	for _, d := range []string{"postgres", "sqlite"} {
		t.Run(d, func(t *testing.T) {
			qb := &QueryBuilder{db: mockDB(d)}
			buildErrContains(t, qb.Upsert("users", upsertValues()).DoUpdate("name").Build(),
				"needs OnConflict(cols...)")
			buildErrContains(t, qb.Upsert("users", upsertValues()).DoUpdateAll().Build(),
				"needs OnConflict(cols...)")
		})
	}
	t.Run("mysql", func(t *testing.T) {
		qb := &QueryBuilder{db: mockDB("mysql")}
		buildOK(t, qb.Upsert("users", upsertValues()).DoUpdate("name").Build(),
			"INSERT INTO `users` (`email`, `id`, `name`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)")
	})
}

func TestUpsert_DoUpdateAll_SQL(t *testing.T) {
	want := map[string]string{
		"postgres": `INSERT INTO "users" ("email", "id", "name") VALUES ($1, $2, $3) ON CONFLICT ("id") DO UPDATE SET "email" = EXCLUDED."email", "name" = EXCLUDED."name"`,
		"sqlite":   `INSERT INTO "users" ("email", "id", "name") VALUES (?, ?, ?) ON CONFLICT ("id") DO UPDATE SET "email" = excluded."email", "name" = excluded."name"`,
		// The target stays out of SET on MySQL too: a row matched through another
		// unique key never gets its id overwritten.
		"mysql": "INSERT INTO `users` (`email`, `id`, `name`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `email` = VALUES(`email`), `name` = VALUES(`name`)",
	}
	for d, sql := range want {
		t.Run(d, func(t *testing.T) {
			qb := &QueryBuilder{db: mockDB(d)}
			q := qb.Upsert("users", upsertValues()).OnConflict("id").DoUpdateAll().Build()
			buildOK(t, q, sql)
			// value ↔ placeholder pairs follow the sorted column order
			if len(q.params) != 3 || q.params[0] != "a@example.com" || q.params[1] != 1 || q.params[2] != "Alice" {
				t.Fatalf("params = %v, want [a@example.com 1 Alice]", q.params)
			}
		})
	}
}

func TestUpsert_DoNothing_SQL(t *testing.T) {
	cases := []struct {
		dialect string
		target  []string
		want    string
	}{
		{"postgres", []string{"id"}, `INSERT INTO "users" ("email", "id", "name") VALUES ($1, $2, $3) ON CONFLICT ("id") DO NOTHING`},
		{"postgres", nil, `INSERT INTO "users" ("email", "id", "name") VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`},
		{"sqlite", nil, `INSERT INTO "users" ("email", "id", "name") VALUES (?, ?, ?) ON CONFLICT DO NOTHING`},
		// MySQL has no DO NOTHING: assign a column to itself. Not INSERT IGNORE,
		// which turns unrelated errors into warnings.
		{"mysql", []string{"id"}, "INSERT INTO `users` (`email`, `id`, `name`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `id` = `id`"},
		{"mysql", nil, "INSERT INTO `users` (`email`, `id`, `name`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `email` = `email`"},
	}
	for _, c := range cases {
		t.Run(c.dialect+"/"+strings.Join(c.target, ","), func(t *testing.T) {
			uq := (&QueryBuilder{db: mockDB(c.dialect)}).Upsert("users", upsertValues())
			if c.target != nil {
				uq = uq.OnConflict(c.target...)
			}
			buildOK(t, uq.DoNothing().Build(), c.want)
		})
	}
}

func TestUpsert_MySQLRowAlias(t *testing.T) {
	db := mockDB("mysql")
	WithMySQLUpsertSyntax(MySQLUpsertRowAlias)(db)
	qb := &QueryBuilder{db: db}
	buildOK(t, qb.Upsert("users", upsertValues()).OnConflict("id").DoUpdate("name").Build(),
		"INSERT INTO `users` (`email`, `id`, `name`) VALUES (?, ?, ?) AS excluded ON DUPLICATE KEY UPDATE `name` = excluded.`name`")
	// DoNothing needs no alias.
	buildOK(t, qb.Upsert("users", upsertValues()).OnConflict("id").DoNothing().Build(),
		"INSERT INTO `users` (`email`, `id`, `name`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `id` = `id`")

	// The option is ignored by the other dialects.
	pg := mockDB("postgres")
	WithMySQLUpsertSyntax(MySQLUpsertRowAlias)(pg)
	buildOK(t, (&QueryBuilder{db: pg}).Upsert("users", upsertValues()).OnConflict("id").DoUpdate("name").Build(),
		`INSERT INTO "users" ("email", "id", "name") VALUES ($1, $2, $3) ON CONFLICT ("id") DO UPDATE SET "name" = EXCLUDED."name"`)
}

// --- BatchInsert -----------------------------------------------------------

func batchRows(qb *QueryBuilder) *BatchInsertQuery {
	return qb.BatchInsert("users", []string{"id", "name"}).Values(1, "A").Values(2, "B")
}

func TestBatchInsert_PlainInsert_NeedsNoAction(t *testing.T) {
	buildOK(t, batchRows(&QueryBuilder{db: mockDB("postgres")}).Build(),
		`INSERT INTO "users" ("id", "name") VALUES ($1, $2), ($3, $4)`)
}

func TestBatchInsert_ConflictRules(t *testing.T) {
	for _, d := range allDialects {
		t.Run(d, func(t *testing.T) {
			qb := &QueryBuilder{db: mockDB(d)}
			buildErrContains(t, batchRows(qb).OnConflict("id").Build(), "exactly one of DoUpdate")
			buildErrContains(t, batchRows(qb).OnConflict("id").DoUpdate("name").DoNothing().Build(), "exactly one of DoUpdate")
			buildErrContains(t, batchRows(qb).OnConflict("id").DoUpdate("email").Build(),
				`DoUpdate column "email" is not among the inserted columns`)
			buildErrContains(t, batchRows(qb).OnConflict("id", "name").DoUpdateAll().Build(), "DoUpdateAll has no column to update")
		})
	}
	// FIX-010 #8: batch DoUpdate without a target was silently dropped.
	for _, d := range []string{"postgres", "sqlite"} {
		t.Run(d+"/DoUpdate without target", func(t *testing.T) {
			buildErrContains(t, batchRows(&QueryBuilder{db: mockDB(d)}).DoUpdate("name").Build(), "needs OnConflict(cols...)")
		})
	}
	t.Run("mysql/DoUpdate without target", func(t *testing.T) {
		buildOK(t, batchRows(&QueryBuilder{db: mockDB("mysql")}).DoUpdate("name").Build(),
			"INSERT INTO `users` (`id`, `name`) VALUES (?, ?), (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)")
	})
}

func TestBatchInsert_Actions_SQL(t *testing.T) {
	buildOK(t, batchRows(&QueryBuilder{db: mockDB("postgres")}).OnConflict("id").DoUpdateAll().Build(),
		`INSERT INTO "users" ("id", "name") VALUES ($1, $2), ($3, $4) ON CONFLICT ("id") DO UPDATE SET "name" = EXCLUDED."name"`)
	buildOK(t, batchRows(&QueryBuilder{db: mockDB("sqlite")}).DoNothing().Build(),
		`INSERT INTO "users" ("id", "name") VALUES (?, ?), (?, ?) ON CONFLICT DO NOTHING`)
	// FIX-010 #2: MySQL DoNothing was a plain INSERT that failed on the duplicate.
	buildOK(t, batchRows(&QueryBuilder{db: mockDB("mysql")}).DoNothing().Build(),
		"INSERT INTO `users` (`id`, `name`) VALUES (?, ?), (?, ?) ON DUPLICATE KEY UPDATE `id` = `id`")
	buildOK(t, batchRows(&QueryBuilder{db: mockDB("mysql")}).OnConflict("name").DoNothing().Build(),
		"INSERT INTO `users` (`id`, `name`) VALUES (?, ?), (?, ?) ON DUPLICATE KEY UPDATE `name` = `name`")

	db := mockDB("mysql")
	WithMySQLUpsertSyntax(MySQLUpsertRowAlias)(db)
	buildOK(t, batchRows(&QueryBuilder{db: db}).OnConflict("id").DoUpdateAll().Build(),
		"INSERT INTO `users` (`id`, `name`) VALUES (?, ?), (?, ?) AS excluded ON DUPLICATE KEY UPDATE `name` = excluded.`name`")
}

// --- Model path --------------------------------------------------------------

type upsertModelRow struct {
	ID        int    `db:"id"`
	Name      string `db:"name"`
	CreatedAt string `db:"created_at"`
}

func (upsertModelRow) TableName() string { return "rows" }

type upsertPKOnly struct {
	ID int `db:"id"`
}

func (upsertPKOnly) TableName() string { return "pk_only" }

func modelFor(d string, m any) *ModelQuery {
	return mockDB(d).Model(m)
}

func TestModelUpsert_ExcludeLeavesColumnOut(t *testing.T) {
	// FIX-010 #9: Exclude was ignored and created_at overwritten on every upsert.
	q, err := modelFor("postgres", &upsertModelRow{ID: 1, Name: "A", CreatedAt: "t0"}).Exclude("created_at").buildUpsert(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	buildOK(t, q, `INSERT INTO "rows" ("id", "name") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "name" = EXCLUDED."name"`)
}

func TestModelUpsert_DefaultAndSelectedFields(t *testing.T) {
	m := &upsertModelRow{ID: 1, Name: "A", CreatedAt: "t0"}
	q, err := modelFor("sqlite", m).buildUpsert(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	buildOK(t, q, `INSERT INTO "rows" ("created_at", "id", "name") VALUES (?, ?, ?) ON CONFLICT ("id") DO UPDATE SET "created_at" = excluded."created_at", "name" = excluded."name"`)

	q, err = modelFor("sqlite", m).buildUpsert(nil, []string{"name"})
	if err != nil {
		t.Fatal(err)
	}
	buildOK(t, q, `INSERT INTO "rows" ("created_at", "id", "name") VALUES (?, ?, ?) ON CONFLICT ("id") DO UPDATE SET "name" = excluded."name"`)

	q, err = modelFor("sqlite", m).buildUpsert([]string{"name"}, []string{"created_at"})
	if err != nil {
		t.Fatal(err)
	}
	buildOK(t, q, `INSERT INTO "rows" ("created_at", "id", "name") VALUES (?, ?, ?) ON CONFLICT ("name") DO UPDATE SET "created_at" = excluded."created_at"`)
}

func TestModelUpsert_InvalidNames_AreErrors(t *testing.T) {
	m := &upsertModelRow{ID: 1, Name: "A"}
	cases := []struct {
		name   string
		mq     *ModelQuery
		target []string
		fields []string
		want   string
	}{
		{"unknown field", modelFor("postgres", m), nil, []string{"title"}, `Upsert field "title" is not a column of the model`},
		{"PK in fields", modelFor("postgres", m), nil, []string{"id"}, `Upsert field "id" is in the conflict target`},
		{"unknown Exclude", modelFor("postgres", m).Exclude("crated_at"), nil, nil, `Exclude name "crated_at" is not a column of the model`},
		{"excluded field", modelFor("postgres", m).Exclude("name"), nil, []string{"name"}, `Upsert field "name" is excluded`},
		{"unknown target", modelFor("postgres", m), []string{"code"}, nil, `UpsertOn conflict column "code" is not a column of the model`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.mq.buildUpsert(c.target, c.fields)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestModelUpsert_NothingToUpdate_IsError(t *testing.T) {
	// FIX-010 #7: a model with only PK columns rendered an empty SET.
	_, err := modelFor("postgres", &upsertPKOnly{ID: 1}).buildUpsert(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "DoUpdateAll has no column to update") {
		t.Fatalf("err = %v", err)
	}
}
