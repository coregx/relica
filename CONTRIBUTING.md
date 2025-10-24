# Contributing to Relica

Thank you for your interest in contributing to Relica! We appreciate your help in making this database query builder better.

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.25+** - [Install Go](https://go.dev/doc/install)
- **golangci-lint** - [Install golangci-lint](https://golangci-lint.run/welcome/install/)
- **Git** - [Install Git](https://git-scm.com/downloads)

### Setup Development Environment

```bash
# Fork the repository on GitHub first!

# Clone your fork
git clone https://github.com/YOUR_USERNAME/relica.git
cd relica

# Add upstream remote
git remote add upstream https://github.com/coregx/relica.git

# Install dependencies
go mod download
go mod verify

# Run tests to verify setup
go test ./...
```

---

## 🛠️ Development Workflow

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./core/...
go test ./cache/...

# Run integration tests (requires databases)
cd test
go test -v ./...
```

### Code Quality

```bash
# Format code
gofmt -w .

# Check formatting
gofmt -l .

# Run static analysis
go vet ./...

# Run linter
golangci-lint run --timeout=5m ./...

# Fix auto-fixable issues
golangci-lint run --fix ./...
```

### Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem ./benchmark/...

# Run specific benchmark
go test -bench=BenchmarkBatchInsert ./benchmark/...

# Compare benchmarks
go test -bench=. -benchmem ./benchmark/... > new.txt
# Make changes...
go test -bench=. -benchmem ./benchmark/... > old.txt
benchstat old.txt new.txt
```

---

## 📋 Before You Submit

### Pre-Commit Checklist

Before submitting a pull request, ensure:

- [ ] Code is formatted: `gofmt -w .`
- [ ] Tests pass: `go test ./...`
- [ ] Linter passes: `golangci-lint run ./...`
- [ ] Coverage maintained or improved
- [ ] Documentation updated (if needed)
- [ ] Examples updated (if API changed)
- [ ] CHANGELOG.md updated (for significant changes)

### Writing Tests

All new code must include tests:

```go
func TestNewFeature(t *testing.T) {
    // Arrange
    db := setupTestDB(t)
    defer db.Close()

    // Act
    result, err := db.Builder()./* your code */

    // Assert
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

**Coverage requirements:**
- **Minimum**: 70% overall coverage
- **Target**: 80%+ for business logic
- **Core package**: 80%+ required

### Code Style

Follow these guidelines:

1. **Naming**:
   - Use `PascalCase` for exported functions/types
   - Use `camelCase` for unexported functions/types
   - Use descriptive names (avoid abbreviations)

2. **Comments**:
   - All exported functions must have godoc comments
   - Comments must start with the function/type name
   - Comments must end with a period
   - Use full sentences

3. **Error Handling**:
   - Always handle errors explicitly
   - Use `fmt.Errorf` with `%w` for error wrapping
   - Return errors, don't panic (except in tests)

4. **Imports**:
   - Group imports: stdlib, external, internal
   - Use `goimports` for automatic formatting

Example:
```go
// BatchInsert creates a batch INSERT query for multiple rows.
// It returns a BatchInsertQuery that can be executed with Execute().
// The columns parameter specifies which columns to insert.
func (qb *QueryBuilder) BatchInsert(table string, columns []string) *BatchInsertQuery {
    if len(columns) == 0 {
        return &BatchInsertQuery{err: ErrNoColumns}
    }
    // ...
}
```

---

## 🔄 Git Workflow

### Branch Naming

Use descriptive branch names:

- `feature/batch-operations` - New features
- `fix/cache-memory-leak` - Bug fixes
- `docs/update-readme` - Documentation updates
- `refactor/query-builder` - Code refactoring
- `test/add-integration-tests` - Test improvements

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, missing semicolons, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**
```bash
git commit -m "feat(batch): add batch UPDATE support"
git commit -m "fix(cache): prevent memory leak in LRU eviction"
git commit -m "docs: update README with UPSERT examples"
git commit -m "test(core): add integration tests for transactions"
```

### Creating a Pull Request

1. **Create feature branch**:
   ```bash
   git checkout develop
   git pull upstream develop
   git checkout -b feature/my-feature
   ```

2. **Make changes and commit**:
   ```bash
   # Make changes...
   gofmt -w .
   go test ./...

   git add .
   git commit -m "feat: add my feature"
   ```

3. **Push to your fork**:
   ```bash
   git push origin feature/my-feature
   ```

4. **Create PR on GitHub**:
   - Go to: https://github.com/coregx/relica/compare
   - Select `develop` as base branch
   - Select your feature branch
   - Fill in PR template
   - Request review

### Pull Request Guidelines

**PR Title**: Use conventional commit format
```
feat(batch): add batch UPDATE support
```

**PR Description** must include:
- **Summary**: What does this PR do?
- **Motivation**: Why is this change needed?
- **Changes**: List of changes made
- **Testing**: How was this tested?
- **Breaking Changes**: Any breaking changes? (if yes, must be major version)

**Example PR Description:**
```markdown
## Summary
Adds batch UPDATE support for updating multiple rows with different values in a single query.

## Motivation
Users need to update many rows efficiently without individual UPDATE queries.

## Changes
- Added BatchUpdateQuery type
- Implemented CASE-WHEN SQL generation for all 3 dialects
- Added 25 unit tests
- Added 8 integration tests
- Updated README with examples

## Testing
- Unit tests: 25 new tests, all passing
- Integration tests: tested on PostgreSQL, MySQL, SQLite
- Benchmarks: 2.5x faster than individual UPDATEs

## Breaking Changes
None - backward compatible addition.
```

---

## 🐛 Reporting Bugs

### Before Reporting

1. Check [existing issues](https://github.com/coregx/relica/issues)
2. Verify it's not already fixed in `develop`
3. Try with latest version

### Bug Report Template

```markdown
**Describe the bug**
A clear description of the bug.

**To Reproduce**
Steps to reproduce:
1. Create DB connection with...
2. Execute query...
3. See error

**Expected behavior**
What you expected to happen.

**Actual behavior**
What actually happened.

**Code Sample**
```go
// Minimal reproducible example
db, _ := relica.Open("postgres", dsn)
// ...
```

**Environment:**
- Relica version: v0.1.0-beta
- Go version: 1.25.0
- OS: Ubuntu 22.04
- Database: PostgreSQL 15

**Additional context**
Any other relevant information.
```

---

## 💡 Feature Requests

### Before Requesting

1. Check [existing issues](https://github.com/coregx/relica/issues)
2. Check [ROADMAP.md](ROADMAP.md) for planned features
3. Consider if it fits Relica's scope (lightweight, zero-dependency)

### Feature Request Template

```markdown
**Problem Statement**
What problem does this feature solve?

**Proposed Solution**
How should this feature work?

**Alternatives Considered**
What other approaches did you consider?

**Use Case**
Real-world example of how this would be used.

**Breaking Changes**
Would this require breaking changes?
```

---

## 📚 Documentation

### Documentation Guidelines

1. **README.md**: High-level overview, quick start, examples
2. **Godoc comments**: All exported functions, types, constants
3. **CHANGELOG.md**: All user-facing changes
4. **docs/reports/**: Detailed implementation reports

### Writing Good Documentation

- Use clear, concise language
- Include code examples
- Explain **why**, not just **what**
- Keep examples up-to-date with code
- Test code examples (they should compile and run)

---

## 📦 Internal Packages

Relica uses Go's `internal/` package structure to maintain a clear public API boundary.

### Understanding Internal Packages

**What you need to know:**

1. **Only import the main package**:
   ```go
   // ✅ Correct - Public API
   import "github.com/coregx/relica"

   // ❌ Wrong - Internal package (will not compile)
   import "github.com/coregx/relica/internal/core"
   ```

2. **All functionality is available** through the main `relica` package via re-exports in `db.go`

3. **Internal packages are not part of the public API**:
   - Changes to `internal/` do NOT require major version bumps
   - We can refactor freely without breaking user code
   - Documentation on pkg.go.dev only shows the public API

### Working with Internal Packages (Contributors)

**When developing Relica:**

- Internal packages CAN import each other
- Tests CAN import internal packages
- Benchmarks CAN import internal packages
- Only `db.go` should re-export to public API

**Example internal import (in contributor code)**:
```go
// internal/core/builder.go
package core

import (
    "github.com/coregx/relica/internal/cache"    // ✅ OK
    "github.com/coregx/relica/internal/dialects" // ✅ OK
)
```

**Adding new public functionality:**

1. Implement in `internal/core/` (or appropriate package)
2. Export through `db.go`:
   ```go
   // db.go
   type (
       NewType = core.NewType  // Re-export
   )

   var (
       NewFunc = core.NewFunc  // Re-export
   )
   ```
3. Document in README.md
4. Add example to docs/

### Why Internal Packages?

- **API Stability**: Changes to implementation don't break user code
- **Freedom to Refactor**: Reorganize code without semver implications
- **Clear Documentation**: pkg.go.dev shows only public API
- **Go Best Practice**: Standard pattern for Go libraries (used by GORM, stdlib packages, etc.)

See [docs/reports/INTERNAL_VS_NO_INTERNAL_2025.md](docs/reports/INTERNAL_VS_NO_INTERNAL_2025.md) for detailed analysis.

---

## 🎯 Release Process

See [RELEASE_GUIDE.md](RELEASE_GUIDE.md) for detailed release process.

**Summary:**
1. Features merged to `develop`
2. Release branch created: `release/v0.2.0`
3. CI passes on release branch
4. Merge to `main` (--no-ff)
5. CI passes on `main`
6. Create tag: `git tag -a v0.2.0`
7. Push tag: `git push origin v0.2.0`
8. Merge back to `develop`

**Only maintainers can create releases.**

---

## 🏆 Recognition

Contributors are recognized in:
- CHANGELOG.md (for significant contributions)
- GitHub contributors page
- Release notes

---

## ❓ Questions?

- **Issues**: [GitHub Issues](https://github.com/coregx/relica/issues)
- **Discussions**: [GitHub Discussions](https://github.com/coregx/relica/discussions)
- **Email**: support@coregx.dev

---

## 📜 Code of Conduct

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community guidelines.

---

**Thank you for contributing to Relica!** 🎉

Every contribution, no matter how small, helps make Relica better for everyone.
