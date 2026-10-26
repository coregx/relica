---
name: relica
description: Working with Relica (github.com/coregx/relica, a zero-dependency SQL query builder for Go, successor of ozzo-dbx) in a project that uses it — find the Relica version in go.mod, read Relica's AGENTS.md at exactly that version, write queries, repositories, transactions and error handling the way it prescribes, audit the project's Relica code with the bundled script (transaction context, error-text matching, hand-written ILIKE, LIKE escaping, SQLite read-only DSNs, and every v0.20.0 behaviour change: OrderBy/Select arguments, UPDATE/DELETE without Where, upsert actions, row locks, column and table naming, Count/Exists, set operations), and upgrade Relica version by version. Use whenever Go code imports github.com/coregx/relica, or the task mentions Relica, ozzo-dbx migration, dbcontext, With(ctx), Transactional, IsUniqueViolation, ILIKE, or a database-layer audit.
license: MIT
allowed-tools: Bash(bash ${CLAUDE_SKILL_DIR}/scripts/audit.sh *)
---

# Relica in a project that uses it

This skill is deliberately thin. The source of truth is Relica's own `AGENTS.md`, `CHANGELOG.md`
and `docs/MIGRATION_GUIDE.md` **at the version the project uses** — not memory, not this file, not
the project's own notes, which go stale. The skill adds what those files cannot: how to get the
right version of them, a scripted audit, and the upgrade procedure.

## 1. Version first

```bash
bash ${CLAUDE_SKILL_DIR}/scripts/audit.sh <repo>     # the version block is printed first
```

It prints the Relica version in every `go.mod` (marks `// indirect`), `replace` lines, `go.work`,
and the latest version on proxy.golang.org. A `replace` or `go.work` entry pointing to a local path
means the code builds against that checkout, not the tag.

Without the plugin (another agent, or no `${CLAUDE_SKILL_DIR}`), the same files ship in the module:

```bash
go list -m -f '{{.Version}} {{.Dir}}' github.com/coregx/relica   # honours go.work and replace
bash <Dir>/skills/relica/scripts/audit.sh <repo>
```

## 2. Read the docs at that version

```bash
go mod download -json github.com/coregx/relica@<version>   # "Dir" = the source at that tag
```

| Task | Read in `<Dir>/AGENTS.md` |
|---|---|
| CRUD, WHERE, joins | "API Priority", "Expression API Reference", "JOIN Operations" |
| Select lists, ORDER BY, GROUP BY | "Ordering with Expressions" (Select takes column names; expressions go through SelectExpr / OrderByExpr / GroupByExpr) |
| Transactions, repositories, dbcontext | "Transactions" (Rules, Which API, Executor, Context propagation, Retrying, Mistakes found in real projects) |
| Errors | "Error Handling" |
| Locks | "Row Locking (FOR UPDATE / FOR SHARE)" |
| UPDATE / DELETE | "UPDATE / DELETE need Where" (v0.20.0+) |
| UNION / INTERSECT / EXCEPT | "Set Operations" (v0.20.0+: OrderBy/Limit after Union apply to the combined result) |
| Names, struct scanning | "Table and Column Names" (v0.20.0+) |
| Count, Exists | "Query Helpers" |

Two sources, two versions:
- **API** (what exists, signatures, rendered SQL): AGENTS.md at the project's version.
- **Guidance and history**: `CHANGELOG.md`, `docs/MIGRATION_GUIDE.md` and the advice sections of
  AGENTS.md from the **latest** version (`@latest` in the same command): they cover every older
  version and get corrections after a release. Use only the API the project's version has, and
  propose the upgrade when a fix needs a newer one.

## 3. Writing code

Follow AGENTS.md. The points that caused real bugs in projects using Relica:
- A dbcontext-style `With(ctx)` returns `relica.Executor`: the `*relica.Tx` kept in ctx, else
  `db.WithContext(ctx)`. `Transactional` joins an outer transaction instead of nesting.
- Every statement of a transaction goes through it; row locks only inside one.
- SQL arithmetic via `relica.NewExp("balance - ?", v)`, never a string value in `Set`.
- Errors by the `relica.Is*` helpers, never by `err.Error()` text; retry the whole transaction, and
  only on `IsDeadlock || IsSerializationFailure`.
- The request context goes into `Begin`/`Transactional`; `context.Background()` only in `main`
  and tests.
- One `*relica.DB` per database, created at startup (`Open`/`WrapDB`), never per request.
- `Like`/`ILike` take the raw value (Relica escapes it); `.EscapeChars()` with no arguments turns
  escaping off — only for a value that is a pattern on purpose.
- SQLite read-only connection: `file:<path>?mode=ro`; on a plain path the drivers drop `mode=ro`.
- `Select`, `GroupBy` take column names and `OrderBy` takes `"column [ASC|DESC]"`, one per
  argument; literals and expressions go through `SelectExpr`, `GroupByExpr`, `OrderByExpr`.

## 4. Audit a project

1. `bash ${CLAUDE_SKILL_DIR}/scripts/audit.sh <repo>` — read-only, git-tracked `*.go` only. It
   reports leads in three groups: mistakes found in real projects (H1–H3, M1–M7), code whose
   behaviour changes in v0.20.0 (V1–V11; the group title says whether the project is still below
   v0.20.0), and style or placement notes (I1–I8). Each line names the AGENTS.md or
   MIGRATION_GUIDE section to read.
2. Every finding is a lead: open the file and confirm before reporting. Then go through the
   script's "Not greppable" list by reading the code.
3. Report per finding: `file:line`, what breaks (a concrete scenario), the fix, the docs section.
4. Code in another repository is that repository's to change: send the findings to its owner.

## 5. Upgrade Relica

1. Read `CHANGELOG.md` and `docs/MIGRATION_GUIDE.md` of the **target** version (step 2) for
   every version between the current and the target one.
2. `GOWORK=off go get github.com/coregx/relica@<target> && GOWORK=off go mod tidy`.
3. Run the audit. For v0.20.0 the V-group lists the code whose behaviour changes; fix those before
   running the application.
4. Behaviour changes that compile silently: for every version you pass, check the project's code
   and tests against each row of that version's MIGRATION_GUIDE section. Do not rely on a list
   copied anywhere else, this file included.
5. `go build ./... && go vet ./... && go test ./...`, plus the project's integration tests on a
   real database — the v0.20.0 struct-scanning rule (`ScanRequireFields`) and the naming rule
   show up only when queries run.

## 6. A Relica bug or gap

Do not work around it silently. Open an issue at https://github.com/coregx/relica/issues with a
minimal reproduction (the builder chain, the dialect, the SQL from `ToSQL()`, the expected SQL).
A workaround in the project is named as such, with a link to the issue.

## Tests of the audit script

`scripts/audit_test.sh` runs `audit.sh` offline on `scripts/testdata/`: `bad/` fires every check a
known number of times, `clean/` holds the correct form of each pattern and must produce nothing.
`bad/` also runs as a git work tree, where an untracked file must not be reported. Fixtures are `*.go.txt` and
`go.mod.txt` so they are neither Go files of the module nor nested modules. Run it after every change to the script;
a new check or a fixed false positive gets its fixture case first. `audit.sh` exits 0 after an audit (findings or
not) and 2 when it cannot run (bash older than 4.3 — macOS: `brew install bash` — or a directory it cannot enter).
