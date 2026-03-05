# Release Guide - Relica

**CRITICAL**: Read this guide BEFORE creating any release!

---

## Backup Before Any Operation

**ALWAYS create a backup before any serious operations!**

```bash
# Create backup BEFORE any git operations with branches/tags
cd /d/projects/relica
cp -r relica relica-backup-$(date +%Y%m%d-%H%M%S)

# Or use git bundle (portable backup)
cd relica
git bundle create ../relica-backup.bundle --all
```

**Dangerous operations (require backup)**:
- `git reset --hard`
- `git branch -D`
- `git tag -d`
- `git push -f`
- `git rebase`
- Any rollback/deletion operations

---

## Git Workflow (GitHub Flow)

### Branch Structure

```
main          - Production-ready code (protected, green CI always)
  ↑
feature/*     - Feature branches (PRs to main)
fix/*         - Bug fix branches (PRs to main)
docs/*        - Documentation branches (PRs to main)
```

**No `develop` branch.** `main` is the single source of truth.

### Branch Rules

#### `main` Branch
- **ALWAYS** production-ready
- **ALWAYS** green CI (all tests passing)
- **ONLY** accepts merges via Pull Requests
- **NEVER** commit directly to main
- **NEVER** push without green CI
- **NEVER** force push
- Tags created ONLY after CI passes on main

#### Feature/Fix Branches
- Created from `main`
- Merged to `main` via PR
- Deleted after merge

---

## Version Naming

### Semantic Versioning

Format: `MAJOR.MINOR.PATCH[-PRERELEASE]`

Examples:
- `v0.10.0` - Minor feature update
- `v0.10.1` - Patch/bugfix
- `v1.0.0` - First major stable release

### Version Increment Rules

**MAJOR** (1.0.0 → 2.0.0):
- Breaking API changes
- Major architectural changes
- Requires migration guide

**MINOR** (1.0.0 → 1.1.0):
- New features (backward compatible)
- New public API methods
- New database dialect support

**PATCH** (1.0.0 → 1.0.1):
- Bug fixes
- Performance improvements
- Documentation updates
- Dependency updates
- Security patches

---

## Pre-Release Checklist

**CRITICAL**: Complete ALL items before tagging!

### 1. Automated Quality Checks

```bash
# Run the pre-release validation script
bash scripts/pre-release.sh
```

This script automatically checks:
- Code formatting (`gofmt`)
- Static analysis (`go vet`)
- Comprehensive linting (`golangci-lint`)
- Module validation (`go mod verify`)
- All tests with coverage
- Benchmark compilation
- TODO/FIXME comments
- Git status

**OR** run checks manually:

```bash
gofmt -w .
gofmt -l .          # Should return nothing
go vet ./...
golangci-lint run --timeout=5m ./...
go test ./... -v
go test -cover ./...
```

### 2. Dependencies
```bash
go mod verify
go mod tidy
git diff go.mod go.sum  # Should show NO changes
```

### 3. Documentation
- [ ] README.md updated with latest features
- [ ] CHANGELOG.md entry created for this version
- [ ] All public APIs have godoc comments
- [ ] Examples are up-to-date and tested

---

## Release Process

### Step 1: Ensure main is up to date

```bash
git checkout main
git pull origin main
git status  # Must be clean
```

### Step 2: Run all checks

```bash
go test ./...
golangci-lint run ./...
go mod verify
```

### Step 3: Verify CI is green

```bash
# Check GitHub Actions
# URL: https://github.com/coregx/relica/actions
# STOP if CI is not GREEN!
```

### Step 4: Triple-check before tagging

```bash
# 1. Verify correct branch and commit
git branch          # Must be on main
git log --oneline -5  # Verify latest commits are correct

# 2. Verify tag order (last tag → new tag)
git tag --sort=-v:refname | head -5  # Check existing tags
# New tag must be the next logical version

# 3. Verify CHANGELOG matches
# Read CHANGELOG.md and confirm it documents everything since last tag
git log --oneline $(git describe --tags --abbrev=0)..HEAD
```

### Step 5: Create annotated tag

```bash
# ONLY after all checks pass!
git tag -a v0.X.X -m "Release v0.X.X

Summary of changes (copy from CHANGELOG)"

# Verify tag
git show v0.X.X
```

### Step 6: Push tag

```bash
git push origin v0.X.X
```

### Step 7: Create GitHub Release

```bash
# Using gh CLI
gh release create v0.X.X --title "v0.X.X - Title" --notes-file tmp/release-notes.md

# Or manually:
# 1. Go to: https://github.com/coregx/relica/releases/new
# 2. Select tag: v0.X.X
# 3. Copy description from CHANGELOG.md
# 4. Publish release
```

---

## Hotfix Process

For critical bugs on `main`:

```bash
# Create fix branch from main
git checkout main
git pull origin main
git checkout -b fix/critical-bug

# Fix the bug, test
go test ./...
golangci-lint run ./...

# Push and create PR to main
git push origin fix/critical-bug
gh pr create --base main

# After PR merged and CI green on main:
git checkout main
git pull origin main

# Triple-check, then tag
git tag -a v0.X.1 -m "Hotfix v0.X.1 - Fix description"
git push origin v0.X.1
```

---

## CI Requirements

All GitHub Actions workflows must be GREEN:

1. **Unit Tests** (3 platforms): Linux, macOS, Windows
2. **Integration Tests**: PostgreSQL 15, MySQL 8
3. **Code Quality**: go vet, golangci-lint, gofmt
4. **Performance**: Benchmarks run successfully
5. **Coverage**: Overall >= 70%, Core >= 80%

---

## NEVER Do This

- **NEVER** commit directly to main
- **NEVER** push to main without green CI
- **NEVER** create tags before CI passes on main
- **NEVER** force push to main
- **NEVER** skip tests or linting (`--no-verify`)

---

## Golden Rules

1. **main = Production** — always green CI, always stable
2. **PRs only** — all changes through Pull Requests
3. **CI first** — NEVER proceed without green CI
4. **Tags last** — only after main CI is green
5. **Triple-check** — verify branch, tag order, CHANGELOG before tagging
6. **Annotated tags** — always use `git tag -a`
7. **No develop** — main is the single source of truth

---

**Remember**: A release can always wait. A broken release cannot be undone.

**When in doubt, wait for CI!**

---

*Last Updated: 2026-03-05*
