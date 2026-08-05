package relica

// One executes a SELECT query and scans a single row into a new value of type T.
// Returns the populated value and any error.
// If no rows match, returns relica.ErrNotFound.
//
// This is the generic equivalent of SelectQuery.One(&dest) — providing
// compile-time type safety for the scan destination.
//
// Example:
//
//	user, err := relica.One[User](db.Select("id", "name").From("users").Where(relica.Eq("id", 1)))
//	if errors.Is(err, relica.ErrNotFound) {
//	    // not found
//	}
func One[T any](sq *SelectQuery) (T, error) {
	var dest T
	err := sq.One(&dest)
	return dest, err
}

// All executes a SELECT query and scans all rows into a slice of type T.
// Returns an empty slice (not nil) if no rows match.
//
// This is the generic equivalent of SelectQuery.All(&dest) — providing
// compile-time type safety for the scan destination.
//
// Example:
//
//	users, err := relica.All[User](
//	    db.Select("id", "name", "email").
//	        From("users").
//	        Where(relica.Eq("status", "active")).
//	        OrderBy("name").
//	        Limit(100),
//	)
func All[T any](sq *SelectQuery) ([]T, error) {
	var dest []T
	err := sq.All(&dest)
	return dest, err
}

// Scalar executes a SELECT query and scans a single scalar value of type T.
// Useful for COUNT, SUM, MAX, MIN, and other aggregate queries.
//
// Example:
//
//	count, err := relica.Scalar[int64](db.Select("COUNT(*)").From("users"))
//	maxPrice, err := relica.Scalar[float64](db.Select("MAX(price)").From("products"))
//	name, err := relica.Scalar[string](db.Select("name").From("users").Where(relica.Eq("id", 1)))
func Scalar[T any](sq *SelectQuery) (T, error) {
	var dest T
	err := sq.Row(&dest)
	return dest, err
}
