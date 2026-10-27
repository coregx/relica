// Driver versions that cannot live in the integration test module: lib/pq
// before v1.10.9 (no SQLState method; its Code field is read by reflection)
// and the cgo driver mattn/go-sqlite3. Run: go test ./... (cgo needed for mattn).
module github.com/coregx/relica/test/drivercompat

go 1.25.0

replace github.com/coregx/relica => ../../

require (
	github.com/coregx/relica v0.0.0
	github.com/lib/pq v1.2.0
	github.com/mattn/go-sqlite3 v1.14.52
)
