# Release Guide - Relica

**CRITICAL**: Read this guide BEFORE creating any release!

---

## 🔴 КРИТИЧЕСКИ ВАЖНО: Backup Before Any Operation

**ВСЕГДА создавай backup перед серьезными операциями!**

```bash
# Создать backup ПЕРЕД любыми git операциями с ветками/тегами
cd /d/projects/relica
cp -r relica relica-backup-$(date +%Y%m%d-%H%M%S)

# Или используй git bundle (переносимый backup)
cd relica
git bundle create ../relica-backup.bundle --all
```

**Опасные операции (требуют backup)**:
- `git reset --hard`
- `git branch -D`
- `git tag -d`
- `git push -f`
- `git rebase`
- Любые операции откатов/удалений

---

## 🎯 Git Flow Strategy

### Branch Structure

```
main        - Production-ready code ONLY (protected, green CI always)
  ↑
release/*   - Release candidates (RC)
  ↑
develop     - Active development (default branch for PRs)
  ↑
feature/*   - Feature branches
```

### Branch Rules

#### `main` Branch
- ✅ **ALWAYS** production-ready
- ✅ **ALWAYS** green CI (all tests passing)
- ✅ **ONLY** accepts merges from `release/*` branches
- ❌ **NEVER** commit directly to main
- ❌ **NEVER** push without green CI
- ❌ **NEVER** force push
- 🏷️ **Tags created ONLY after CI passes**

#### `develop` Branch
- Default branch for development
- Accepts feature branches
- May contain work-in-progress code
- Should pass tests, but can have warnings

#### `release/*` Branches
- Format: `release/v0.1.0-beta`, `release/v1.0.0`
- Created from `develop`
- Only bug fixes and documentation updates allowed
- No new features
- Merges to both `main` and `develop`

#### `feature/*` Branches
- Format: `feature/batch-operations`, `feature/transaction-support`
- Created from `develop`
- Merged back to `develop` via PR

---

## 📋 Version Naming

### Semantic Versioning

Format: `MAJOR.MINOR.PATCH[-PRERELEASE]`

Examples:
- `v0.1.0-alpha` - First alpha release
- `v0.1.0-beta` - First beta release
- `v0.1.0-rc.1` - Release candidate 1
- `v0.1.0` - First stable minor release
- `v1.0.0` - First major stable release
- `v1.1.0` - Minor feature update
- `v1.1.1` - Patch/bugfix

### Version Increment Rules

**MAJOR** (1.0.0 → 2.0.0):
- Breaking API changes
- Major architectural changes
- Requires migration guide

**MINOR** (1.0.0 → 1.1.0):
- New features (backward compatible)
- Significant improvements
- New database dialect support

**PATCH** (1.0.0 → 1.0.1):
- Bug fixes
- Performance improvements
- Documentation updates
- Security patches

**PRERELEASE**:
- `-alpha` - Early testing, unstable API
- `-beta` - Feature complete, testing phase
- `-rc.N` - Release candidate (N = 1, 2, 3...)

---

## ✅ Pre-Release Checklist

**CRITICAL**: Complete ALL items before creating release branch!

### 1. Automated Quality Checks

**Run the pre-release validation script** (recommended):

```bash
# Linux/Mac
./scripts/pre-release.sh

# Windows (requires Git Bash)
scripts\pre-release.bat
```

This script automatically checks:
- ✅ Code formatting (`gofmt`)
- ✅ Static analysis (`go vet`)
- ✅ Comprehensive linting (`golangci-lint`)
- ✅ Module validation (`go mod verify`)
- ✅ All tests with coverage
- ✅ Benchmark compilation
- ✅ TODO/FIXME comments
- ✅ Git status

**Exit codes:**
- `0` - All checks passed or only warnings
- `1` - Errors found, must fix before release

**OR** run checks manually:

<details>
<summary>Click to see manual checks</summary>

```bash
# Format code
gofmt -w .
gofmt -l .  # Should return nothing

# Static analysis
go vet ./...

# Linting
golangci-lint run --timeout=5m ./...

# Unit tests
go test ./... -v

# With coverage
go test -cover ./...
# Minimum: 70% overall, 80% for core

# Benchmarks
go test -bench=. -benchmem ./benchmark/...
```

</details>

### 2. Dependencies
```bash
# Verify modules
go mod verify

# Tidy and check diff
go mod tidy
git diff go.mod go.sum
# Should show NO changes
```

### 3. Documentation
- [ ] README.md updated with latest features
- [ ] CHANGELOG.md entry created for this version
- [ ] All public APIs have godoc comments
- [ ] Examples are up-to-date and tested
- [ ] Migration guide (if breaking changes)

### 4. GitHub Actions
- [ ] `.github/workflows/test.yml` exists
- [ ] All workflows tested on `develop`
- [ ] CI passes on latest `develop` commit

---

## 🚀 Release Process

### Step 1: Create Release Branch

```bash
# Ensure you're on develop and up-to-date
git checkout develop
git pull origin develop

# Verify develop is clean
git status

# Create release branch (example: v0.1.0-beta)
git checkout -b release/v0.1.0-beta

# Update version in files if needed
# (Update README.md badges, CHANGELOG.md, etc.)

git add .
git commit -m "chore: prepare v0.1.0-beta release"
git push origin release/v0.1.0-beta
```

### Step 2: Wait for CI (CRITICAL!)

```bash
# Push release branch to GitHub
git push origin release/v0.1.0-beta

# Go to GitHub Actions and WAIT for green CI
# URL: https://github.com/coregx/relica/actions
```

**⏸️ STOP HERE! Do NOT proceed until CI is GREEN!**

✅ **All checks must pass:**
- Unit tests (Linux, macOS, Windows)
- Integration tests (PostgreSQL, MySQL)
- Linting
- Benchmarks
- Code formatting
- Coverage check

❌ **If CI fails:**
1. Fix issues in `release/v0.1.0-beta` branch
2. Commit fixes
3. Push and wait for CI again
4. Repeat until GREEN

### Step 3: Merge to Main (After Green CI)

```bash
# ONLY after CI is green!
git checkout main
git pull origin main

# Merge release branch (--no-ff ensures merge commit)
git merge --no-ff release/v0.1.0-beta -m "Release v0.1.0-beta

Complete v0.1.0-beta implementation:
- Full CRUD operations (SELECT, INSERT, UPDATE, DELETE, UPSERT)
- Transaction support with all isolation levels
- High-performance batch operations (3.3x faster)
- LRU statement cache with metrics
- Zero production dependencies
- Context propagation throughout
- 3 database dialects (PostgreSQL, MySQL, SQLite)
- Comprehensive test suite (123+ tests, 83% coverage)
- Production-ready documentation"

# Push to main
git push origin main
```

### Step 4: Wait for CI on Main

```bash
# Go to GitHub Actions and verify main branch CI
# https://github.com/coregx/relica/actions

# WAIT for green CI on main branch!
```

**⏸️ STOP! Do NOT create tag until main CI is GREEN!**

### Step 5: Create Tag (After Green CI on Main)

```bash
# ONLY after main CI is green!

# Create annotated tag
git tag -a v0.1.0-beta -m "Release v0.1.0-beta

Relica v0.1.0-beta - Lightweight, Type-Safe Database Query Builder

Features:
- Full CRUD operations (SELECT, INSERT, UPDATE, DELETE, UPSERT)
- Transaction support with all 4 isolation levels
- High-performance batch operations (3.3x faster than individual ops)
- LRU statement cache (sub-60ns hits, configurable capacity)
- Zero production dependencies
- Context propagation throughout query chain
- 3 database dialects: PostgreSQL, MySQL, SQLite

Performance:
- Batch INSERT: 3.3x faster (100 rows: 302ms vs 1033ms)
- Batch UPDATE: 2.5x faster (100 rows: 2017ms vs baseline)
- Statement cache: <60ns hit latency
- 83% test coverage (98.1% for cache)

Quality:
- 123+ unit tests passing
- Integration tests for all 3 databases
- golangci-lint compliant
- Production-ready documentation

Zero Dependencies:
- Production code uses only Go standard library
- Test dependencies isolated in separate module
- Binary size: minimal overhead (+50KB)

See CHANGELOG.md for complete details."

# Push tag
git push origin v0.1.0-beta
```

### Step 6: Merge Back to Develop

```bash
# Keep develop in sync
git checkout develop
git merge --no-ff release/v0.1.0-beta -m "Merge release v0.1.0-beta back to develop"
git push origin develop

# Delete release branch (optional, after confirming release is good)
git branch -d release/v0.1.0-beta
git push origin --delete release/v0.1.0-beta
```

### Step 7: Create GitHub Release

1. Go to: https://github.com/coregx/relica/releases/new
2. Select tag: `v0.1.0-beta`
3. Release title: `v0.1.0-beta - Lightweight, Type-Safe Database Query Builder`
4. Description: Copy from CHANGELOG.md
5. Check "Set as a pre-release" if beta/alpha/rc
6. Click "Publish release"

---

## 🔥 Hotfix Process

For critical bugs in production (`main` branch):

```bash
# Create hotfix branch from main
git checkout main
git pull origin main
git checkout -b hotfix/v1.0.1

# Fix the bug
# ... make changes ...

# Test thoroughly
go test ./...
golangci-lint run ./...

# Commit
git add .
git commit -m "fix: critical bug in transaction rollback"

# Push and wait for CI
git push origin hotfix/v1.0.1

# WAIT FOR GREEN CI!

# Merge to main
git checkout main
git merge --no-ff hotfix/v1.0.1 -m "Hotfix v1.0.1"
git push origin main

# WAIT FOR GREEN CI ON MAIN!

# Create tag
git tag -a v1.0.1 -m "Hotfix v1.0.1 - Fix critical transaction bug"
git push origin v1.0.1

# Merge back to develop
git checkout develop
git merge --no-ff hotfix/v1.0.1 -m "Merge hotfix v1.0.1"
git push origin develop

# Delete hotfix branch
git branch -d hotfix/v1.0.1
git push origin --delete hotfix/v1.0.1
```

---

## 📊 CI Requirements

### Must Pass Before Release

All GitHub Actions workflows must be GREEN:

1. **Unit Tests** (3 platforms)
   - Linux (ubuntu-latest)
   - macOS (macos-latest)
   - Windows (windows-latest)

2. **Integration Tests**
   - PostgreSQL 15
   - MySQL 8

3. **Code Quality**
   - go vet (no errors)
   - golangci-lint (pass or warnings only)
   - gofmt (all files formatted)

4. **Performance**
   - Benchmarks run successfully
   - No performance regressions

5. **Coverage**
   - Overall: ≥70%
   - Core package: ≥80%

---

## 🚫 NEVER Do This

❌ **NEVER commit directly to main**
```bash
# WRONG!
git checkout main
git commit -m "quick fix"  # ❌ NO!
```

❌ **NEVER push to main without green CI**
```bash
# WRONG!
git push origin main  # ❌ WAIT for CI first!
```

❌ **NEVER create tags before CI passes**
```bash
# WRONG!
git tag v1.0.0  # ❌ WAIT for green CI on main!
git push origin v1.0.0
```

❌ **NEVER force push to main or develop**
```bash
# WRONG!
git push -f origin main  # ❌ NEVER!
```

❌ **NEVER skip tests or linting**
```bash
# WRONG!
git commit -m "skip CI" --no-verify  # ❌ NO!
```

---

## ✅ Always Do This

✅ **ALWAYS wait for green CI before proceeding**
```bash
# Correct workflow:
git push origin release/v0.1.0-beta
# ⏸️ WAIT for green CI
git checkout main
git merge --no-ff release/v0.1.0-beta
git push origin main
# ⏸️ WAIT for green CI on main
git tag -a v0.1.0-beta -m "..."
git push origin v0.1.0-beta
```

✅ **ALWAYS use annotated tags**
```bash
# Good
git tag -a v1.0.0 -m "Release v1.0.0"

# Bad
git tag v1.0.0  # Lightweight tag
```

✅ **ALWAYS update CHANGELOG.md**
- Document all changes
- Include breaking changes
- Add migration notes

✅ **ALWAYS test on all platforms locally if possible**
```bash
# At minimum:
go test ./...
golangci-lint run ./...
go mod verify
```

---

## 📝 Release Checklist Template

Copy this for each release:

```markdown
## Release vX.Y.Z Checklist

### Pre-Release
- [ ] All tests passing locally
- [ ] Code formatted (gofmt -w .)
- [ ] Linter clean (golangci-lint run)
- [ ] Dependencies verified (go mod verify)
- [ ] CHANGELOG.md updated
- [ ] README.md updated (if needed)
- [ ] Version bumped in relevant files

### Release Branch
- [ ] Created release/vX.Y.Z from develop
- [ ] Pushed to GitHub
- [ ] CI GREEN on release branch
- [ ] All checks passed

### Main Branch
- [ ] Merged release branch to main
- [ ] Pushed to origin
- [ ] CI GREEN on main
- [ ] All checks passed

### Tagging
- [ ] Created annotated tag vX.Y.Z
- [ ] Tag message includes full changelog
- [ ] Pushed tag to origin
- [ ] GitHub release created

### Cleanup
- [ ] Merged back to develop
- [ ] Deleted release branch
- [ ] Verified pkg.go.dev updated
- [ ] Announced release (if applicable)
```

---

## 🎯 Summary: Golden Rules

1. **main = Production ONLY** - Always green CI, always stable
2. **Wait for CI** - NEVER proceed without green CI
3. **Tags LAST** - Only after main CI is green
4. **No Direct Commits** - Use release branches
5. **Annotated Tags** - Always use `git tag -a`
6. **Full Testing** - Run all checks before release branch
7. **Document Everything** - Update CHANGELOG.md, README.md
8. **Git Flow** - develop → release/* → main → tag

---

**Remember**: A release can always wait. A broken production release cannot be undone.

**When in doubt, wait for CI!**

---

*Last Updated: 2025-10-24*
*Relica v0.1.0-beta Release Process*
