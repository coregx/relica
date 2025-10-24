# Scripts

This directory contains automation scripts for Relica development.

## Pre-Release Validation

Run comprehensive quality checks before creating a release.

### Linux/Mac

```bash
./scripts/pre-release.sh
```

### Windows

```cmd
scripts\pre-release.bat
```

## What It Checks

The pre-release script performs the following validations:

1. **Code Formatting** (`gofmt`)
   - Ensures all Go files are properly formatted
   - Exits with error if files need formatting

2. **Static Analysis** (`go vet`)
   - Runs Go's built-in static analysis
   - Detects common Go programming errors

3. **Linting** (`golangci-lint`)
   - Runs comprehensive linter with 40+ checkers
   - Non-blocking (warnings only) for test files

4. **Module Validation** (`go mod`)
   - Runs `go mod tidy`
   - Verifies all modules

5. **Test Suite** (`go test`)
   - Runs all unit and integration tests
   - Displays coverage information
   - Requires minimum 70% overall coverage

6. **Benchmarks** (`go test -bench`)
   - Verifies benchmarks compile successfully

7. **Code Comments**
   - Checks for TODO/FIXME comments
   - Warns if found (informational only)

8. **Git Status**
   - Checks for uncommitted changes
   - Warns if working directory is dirty

## Exit Codes

- **0** - All checks passed or only warnings
- **1** - One or more checks failed with errors

## Usage in CI/CD

The script is designed to be run in CI/CD pipelines:

```yaml
# .github/workflows/release.yml
- name: Pre-release validation
  run: ./scripts/pre-release.sh
```

## Requirements

- Go 1.25+
- `golangci-lint` (optional, but recommended)
  - Install: https://golangci-lint.run/welcome/install/
- Git (for status check)

## Output Example

```
========================================
  Relica Pre-Release Validation
========================================

[INFO] Checking Go version...
[SUCCESS] Go version: go1.25.3

[INFO] Checking code formatting (gofmt)...
[SUCCESS] All files are properly formatted

[INFO] Running go vet...
[SUCCESS] go vet passed

[INFO] Running golangci-lint...
[SUCCESS] golangci-lint passed with no issues

[INFO] Validating go.mod...
[SUCCESS] go.mod is valid

[INFO] Running tests...
[SUCCESS] All tests passed

[INFO] Checking test coverage...
[INFO] Test coverage: 83.0%
[SUCCESS] Coverage meets minimum requirement (70%)

[INFO] Verifying benchmarks compile...
[SUCCESS] Benchmarks compile successfully

[INFO] Checking for TODO/FIXME comments...
[SUCCESS] No TODO/FIXME comments found

[INFO] Checking git status...
[SUCCESS] Working directory is clean

========================================
  Summary
========================================

[SUCCESS] All checks passed! Ready for release.
```

## Troubleshooting

### gofmt errors

```bash
# Fix formatting
gofmt -w .
```

### golangci-lint not found

```bash
# Install golangci-lint
# See: https://golangci-lint.run/welcome/install/

# macOS
brew install golangci-lint

# Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Windows
# Download from: https://github.com/golangci/golangci-lint/releases
```

### Tests failing

```bash
# Run tests with verbose output
go test -v ./...

# Run specific test
go test -v -run TestName ./...
```

### Coverage below 70%

Add more tests to improve coverage. Focus on:
- Uncovered code paths
- Error handling
- Edge cases

```bash
# Check coverage by package
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## See Also

- [CONTRIBUTING.md](../CONTRIBUTING.md) - Development guidelines
- [RELEASE_GUIDE.md](../RELEASE_GUIDE.md) - Release process
- [.golangci.yml](../.golangci.yml) - Linter configuration
