package dialects

import "testing"

// UpsertSQL only renders; validation (ADR-019) lives in core. Column names
// arrive quoted.
func TestUpsertSQL(t *testing.T) {
	pg, my, lite := &PostgresDialect{}, &MySQLDialect{}, &SQLiteDialect{}
	tests := []struct {
		name string
		d    Dialect
		u    Upsert
		want string
	}{
		{"pg do nothing with target", pg, Upsert{Target: []string{`"id"`}, DoNothing: true}, ` ON CONFLICT ("id") DO NOTHING`},
		{"pg do nothing any constraint", pg, Upsert{DoNothing: true}, ` ON CONFLICT DO NOTHING`},
		{"pg do update", pg, Upsert{Target: []string{`"a"`, `"b"`}, Update: []string{`"x"`, `"y"`}},
			` ON CONFLICT ("a", "b") DO UPDATE SET "x" = EXCLUDED."x", "y" = EXCLUDED."y"`},
		{"pg ignores RowAlias", pg, Upsert{Target: []string{`"id"`}, Update: []string{`"x"`}, RowAlias: true},
			` ON CONFLICT ("id") DO UPDATE SET "x" = EXCLUDED."x"`},
		{"sqlite do nothing", lite, Upsert{DoNothing: true}, ` ON CONFLICT DO NOTHING`},
		{"sqlite do update", lite, Upsert{Target: []string{`"id"`}, Update: []string{`"x"`}},
			` ON CONFLICT ("id") DO UPDATE SET "x" = excluded."x"`},
		{"mysql do nothing is a self-assignment", my, Upsert{Target: []string{"`id`"}, DoNothing: true, NoOp: "`id`"},
			" ON DUPLICATE KEY UPDATE `id` = `id`"},
		{"mysql do update VALUES()", my, Upsert{Update: []string{"`x`", "`y`"}},
			" ON DUPLICATE KEY UPDATE `x` = VALUES(`x`), `y` = VALUES(`y`)"},
		{"mysql do update row alias", my, Upsert{Update: []string{"`x`"}, RowAlias: true},
			" AS excluded ON DUPLICATE KEY UPDATE `x` = excluded.`x`"},
		{"mysql do nothing ignores RowAlias", my, Upsert{DoNothing: true, NoOp: "`id`", RowAlias: true},
			" ON DUPLICATE KEY UPDATE `id` = `id`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.UpsertSQL(tt.u); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}
