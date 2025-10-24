#!/bin/bash
# Pre-Release Validation Script for Relica
# This script runs all quality checks before creating a release

set -e  # Exit on first error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Header
echo ""
echo "========================================"
echo "  Relica Pre-Release Validation"
echo "========================================"
echo ""

# Track overall status
ERRORS=0
WARNINGS=0

# 1. Check Go version
log_info "Checking Go version..."
GO_VERSION=$(go version | awk '{print $3}')
log_success "Go version: $GO_VERSION"
echo ""

# 2. Validate golangci-lint configuration
log_info "Validating golangci-lint configuration..."
if command -v golangci-lint &> /dev/null; then
    if golangci-lint config verify; then
        log_success "golangci-lint configuration is valid"
    else
        log_error "golangci-lint configuration validation failed"
        ERRORS=$((ERRORS + 1))
    fi
else
    log_warning "golangci-lint not installed, skipping config validation"
    log_info "Install: https://golangci-lint.run/welcome/install/"
    WARNINGS=$((WARNINGS + 1))
fi
echo ""

# 3. Code formatting check
log_info "Checking code formatting (gofmt)..."
UNFORMATTED=$(gofmt -l .)
if [ -n "$UNFORMATTED" ]; then
    log_error "The following files need formatting:"
    echo "$UNFORMATTED"
    echo ""
    log_info "Run: gofmt -w ."
    ERRORS=$((ERRORS + 1))
else
    log_success "All files are properly formatted"
fi
echo ""

# 4. Go vet
log_info "Running go vet..."
if go vet ./...; then
    log_success "go vet passed"
else
    log_error "go vet failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 5. golangci-lint
log_info "Running golangci-lint..."
if command -v golangci-lint &> /dev/null; then
    if golangci-lint run --timeout=5m ./...; then
        log_success "golangci-lint passed with no issues"
    else
        log_warning "golangci-lint found issues (non-blocking in CI)"
        WARNINGS=$((WARNINGS + 1))
    fi
else
    log_warning "golangci-lint not installed, skipping"
    log_info "Install: https://golangci-lint.run/welcome/install/"
    WARNINGS=$((WARNINGS + 1))
fi
echo ""

# 6. go.mod validation
log_info "Validating go.mod..."
go mod tidy
if go mod verify; then
    log_success "go.mod is valid"
else
    log_error "go.mod verification failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 7. Run tests
log_info "Running tests..."
if go test -v -cover ./...; then
    log_success "All tests passed"
else
    log_error "Tests failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 8. Test coverage check
log_info "Checking test coverage..."
# Get coverage for internal/core (main package)
COVERAGE=$(go test -cover ./... 2>&1 | grep "internal/core" | awk '{print $5}' | sed 's/%//')
if [ -n "$COVERAGE" ]; then
    log_info "Test coverage (internal/core): ${COVERAGE}%"
    # Check if coverage is above 70% (using awk instead of bc for portability)
    if awk -v cov="$COVERAGE" 'BEGIN {exit !(cov >= 70.0)}'; then
        log_success "Coverage meets minimum requirement (70%)"
    else
        log_warning "Coverage below 70% (current: ${COVERAGE}%)"
        WARNINGS=$((WARNINGS + 1))
    fi
else
    log_warning "Could not determine coverage for internal/core"
    WARNINGS=$((WARNINGS + 1))
fi
echo ""

# 9. Run benchmarks (optional, just verify they compile)
log_info "Verifying benchmarks compile..."
if go test -bench=. -run=^$ ./benchmark/... > /dev/null 2>&1; then
    log_success "Benchmarks compile successfully"
else
    log_warning "Benchmark compilation issues"
    WARNINGS=$((WARNINGS + 1))
fi
echo ""

# 10. Check for TODO/FIXME comments
log_info "Checking for TODO/FIXME comments..."
TODO_COUNT=$(grep -r "TODO\|FIXME" --include="*.go" . | wc -l)
if [ "$TODO_COUNT" -gt 0 ]; then
    log_warning "Found $TODO_COUNT TODO/FIXME comments"
    WARNINGS=$((WARNINGS + 1))
else
    log_success "No TODO/FIXME comments found"
fi
echo ""

# 11. Check git status
log_info "Checking git status..."
if git diff-index --quiet HEAD --; then
    log_success "Working directory is clean"
else
    log_warning "Uncommitted changes detected"
    git status --short
    WARNINGS=$((WARNINGS + 1))
fi
echo ""

# Summary
echo "========================================"
echo "  Summary"
echo "========================================"
echo ""

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
    log_success "All checks passed! Ready for release."
    echo ""
    exit 0
elif [ $ERRORS -eq 0 ]; then
    log_warning "Checks completed with $WARNINGS warning(s)"
    echo ""
    log_info "Review warnings above before proceeding with release"
    exit 0
else
    log_error "Checks failed with $ERRORS error(s) and $WARNINGS warning(s)"
    echo ""
    log_error "Fix errors before creating release"
    exit 1
fi
