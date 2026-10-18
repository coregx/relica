// Package drivercompat checks Relica's error classification against driver
// versions that cannot share the integration test module: lib/pq v1.2.0 (code
// in a struct field, no SQLState method) and the cgo driver mattn/go-sqlite3.
package drivercompat
