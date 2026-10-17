package relica

import (
	"database/sql"

	"github.com/coregx/relica/internal/core"
)

// Params holds values for named placeholders written as {:name}:
//
//	db.Select().From("users").
//	    Where("status = {:status} AND (owner_id = {:id} OR author_id = {:id})",
//	        relica.Params{"status": "active", "id": userID}).
//	    All(&users)
//
//	db.NewQuery("SELECT * FROM users WHERE id = {:id}").
//	    BindParams(relica.Params{"id": 1}).
//	    One(&user)
//
// Pass Params as the only argument after a condition string in Where,
// AndWhere, OrWhere or Having. A name may repeat; each occurrence becomes its
// own positional placeholder. A name missing from Params makes the query fail
// with an error naming it.
//
// The {:name} form never collides with SQL syntax (PostgreSQL :: casts, MySQL
// @variables, JSONB ? operators).
type Params map[string]any

// NullStringMap is one row scanned without a struct: column name to
// sql.NullString. Use it as the destination of One, All, Query.One and the
// generic One/All when the columns are not known at compile time.
//
// Example:
//
//	var row relica.NullStringMap
//	err := db.Select().From("users").Where(relica.Eq("id", 1)).One(&row)
//	name := row.String("name") // "" if NULL or missing
//	if !row.IsNull("email") {
//	    email := row.String("email")
//	}
//
//	var rows []relica.NullStringMap
//	err = db.Select().From("users").All(&rows)
type NullStringMap map[string]sql.NullString

// String returns the value of column key, or "" if it is NULL or absent.
func (m NullStringMap) String(key string) string { return core.NullStringMap(m).String(key) }

// IsNull reports whether column key is NULL or absent.
func (m NullStringMap) IsNull(key string) bool { return core.NullStringMap(m).IsNull(key) }

// Has reports whether column key is present (NULL or not).
func (m NullStringMap) Has(key string) bool { return core.NullStringMap(m).Has(key) }

// Keys returns the column names in sorted order.
func (m NullStringMap) Keys() []string { return core.NullStringMap(m).Keys() }

// Get returns the raw sql.NullString for column key and whether it is present.
func (m NullStringMap) Get(key string) (sql.NullString, bool) { return core.NullStringMap(m).Get(key) }
