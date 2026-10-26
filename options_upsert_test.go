package relica

import (
	"testing"

	"github.com/coregx/relica/internal/core"
)

// WithMySQLUpsertSyntax converts the public value to core's by number.
func TestMySQLUpsertSyntax_MatchesCore(t *testing.T) {
	if int(MySQLUpsertValues) != int(core.MySQLUpsertValues) || int(MySQLUpsertRowAlias) != int(core.MySQLUpsertRowAlias) {
		t.Fatal("public MySQLUpsertSyntax constants drifted from internal/core")
	}
	if WithMySQLUpsertSyntax(MySQLUpsertRowAlias).apply == nil {
		t.Fatal("WithMySQLUpsertSyntax returned the zero Option")
	}
}
