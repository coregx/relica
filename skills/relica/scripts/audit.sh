#!/usr/bin/env bash
# audit.sh — Relica version check and audit of a consumer repository (read-only).
#
# Usage: bash audit.sh [repo-dir]        (default: current directory)
#
# Prints the Relica version each go.mod requires (and any replace / go.work
# override), the latest published version, then grep findings: mistakes found
# in real projects, and code whose behaviour changed in v0.20.0. Every finding
# is a lead to read, not a verdict: open the file, read the code, then decide.
# Exit code: 0 after an audit, with findings or not; 2 when it cannot run
# (bash older than 4.3, a directory it cannot enter).
#
# Scans git-tracked *.go files (or every *.go outside vendor/ and tmp/ when the
# directory is not a git work tree). Never modifies anything.
#
# Environment: RELICA_AUDIT_OFFLINE=1 skips the proxy.golang.org lookup.

set -u
# mapfile and local -n need bash 4.3; macOS ships 3.2, where they fail per
# command and the audit would "succeed" with nothing found.
if [ "${BASH_VERSINFO[0]:-0}" -lt 4 ] || { [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -lt 3 ]; }; then
	echo "audit: needs bash 4.3+ (macOS: brew install bash), this is ${BASH_VERSION:-not bash}" >&2
	exit 2
fi
repo="${1:-.}"
cd "$repo" || {
	echo "audit: cannot cd to $repo"
	exit 2
}

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	mapfile -t gofiles < <(git ls-files '*.go' | grep -v '^vendor/')
	mapfile -t gomods < <(git ls-files 'go.mod' '*/go.mod' | grep -v '^vendor/')
else
	mapfile -t gofiles < <(find . -name '*.go' -not -path './vendor/*' -not -path './tmp/*' | sed 's|^\./||')
	mapfile -t gomods < <(find . -name go.mod -not -path './vendor/*' -not -path './tmp/*' | sed 's|^\./||')
fi
mapfile -t prodfiles < <(printf '%s\n' "${gofiles[@]}" | grep -v '_test\.go$' | grep -v '^$')
mapfile -t testfiles < <(printf '%s\n' "${gofiles[@]}" | grep '_test\.go$')
# Files that use another query builder or ORM (ozzo-dbx, GORM, squirrel, bun,
# ent) and not Relica: the same method names (Select, OrderBy, Delete…) follow
# that library's rules there, so the Relica-specific checks skip them. Files
# importing none of them (a wrapper type) stay in.
otherlibs='"(github\.com/go-ozzo/ozzo-dbx|gorm\.io/gorm|github\.com/Masterminds/squirrel|github\.com/uptrace/bun|entgo\.io/ent)[/"]'
mapfile -t otheronly < <(
	[ ${#prodfiles[@]} -gt 0 ] && printf '%s\0' "${prodfiles[@]}" |
		xargs -0 grep -lE "$otherlibs" 2>/dev/null |
		while read -r f; do grep -q '"github.com/coregx/relica"' "$f" || echo "$f"; done
)
mapfile -t relicafiles < <(printf '%s\n' "${prodfiles[@]}" | grep -vxF -f <(printf '%s\n' "${otheronly[@]}") | grep -v '^$')
# context.Background() is legitimate in main packages and CLI tools.
mapfile -t servicefiles < <(printf '%s\n' "${prodfiles[@]}" | grep -vE '(^|/)cmd/|(^|/)main\.go$' | grep -v '^$')

echo "== Relica version ($(pwd))"
found=0
below020=0
for m in "${gomods[@]}"; do
	line=$(grep -E '^\s*(require\s+)?github\.com/coregx/relica v' "$m" | head -1)
	[ -n "$line" ] || continue
	found=1
	v=$(printf '%s' "$line" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+[^[:space:]]*')
	minor=$(printf '%s' "$v" | sed -E 's/^v0\.([0-9]+)\..*/\1/')
	case $v in v0.*) [ "$minor" -lt 20 ] 2>/dev/null && below020=1 ;; esac
	case $line in *indirect*) v="$v (indirect)" ;; esac
	echo "  $m: $v"
	# One-line replace, or a line of a replace ( … ) block, with or without the
	# version on the left ("github.com/coregx/relica v0.19.1 => ../relica").
	grep -nE 'replace[[:space:]]+github\.com/coregx/relica|^[[:space:]]*github\.com/coregx/relica([[:space:]]+v[^[:space:]]+)?[[:space:]]*=>' "$m" |
		sed 's/^/    replace: /'
done
[ -f go.work ] && grep -nE 'coregx/relica|^\s*use|^use' go.work | sed 's/^/  go.work: /'
[ "$found" = 1 ] || echo "  no go.mod requires github.com/coregx/relica"
if [ "${RELICA_AUDIT_OFFLINE:-}" = 1 ]; then
	echo "  latest on proxy.golang.org: not checked (RELICA_AUDIT_OFFLINE=1)"
else
	tmpd=$(mktemp -d)
	latest=$(cd "$tmpd" && GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list -m -f '{{.Version}}' github.com/coregx/relica@latest 2>/dev/null)
	rmdir "$tmpd" 2>/dev/null
	echo "  latest on proxy.golang.org: ${latest:-unknown (offline?)}"
fi
echo

high=0 medium=0 info=0
# report SEVERITY ID TITLE WHERE — reads "file:line:code" lines on stdin.
report() {
	local sev=$1 id=$2 title=$3 where=$4 lines
	lines=$(cat)
	[ -n "$lines" ] || return 0
	local n
	n=$(printf '%s\n' "$lines" | wc -l)
	case $sev in HIGH) high=$((high + n)) ;; MEDIUM) medium=$((medium + n)) ;; *) info=$((info + n)) ;; esac
	echo "[$sev] $id $title ($n)"
	echo "       see: $where"
	local show=$n
	[ "$sev" = INFO ] && [ "$n" -gt 5 ] && show=5
	printf '%s\n' "$lines" | head -n "$show" | sed -E 's/[[:space:]]+/ /g' | cut -c1-200 | sed 's/^/       /'
	[ "$show" -lt "$n" ] && echo "       … $((n - show)) more"
	echo
}
g() { # g FILESET-NAME ERE — grep -nE over the chosen file set
	local -n set=$1
	[ ${#set[@]} -gt 0 ] || return 0
	printf '%s\0' "${set[@]}" | xargs -0 grep -HnE -- "$2" 2>/dev/null # -H: file name even for one file
}
awk_files() { # awk_files FILESET-NAME PROGRAM — run awk over the chosen file set
	local -n set=$1
	[ ${#set[@]} -gt 0 ] || return 0
	printf '%s\0' "${set[@]}" | xargs -0 awk "$2" 2>/dev/null
}

echo "== Findings (leads, verify each by reading the code)"
echo

rollback_before_err() {
	awk_files prodfiles '
		FNR == 1 { begin = 0 }
		/[:=][[:space:]]*[A-Za-z_.]+\.Begin(Tx)?\(/ { begin = FNR; code = $0; next }
		begin && FNR == begin + 1 && /defer .*\.Rollback\(\)/ { print FILENAME ":" begin ":" code }
		{ begin = 0 }'
}
error_text() { # comparisons on err.Error() only, not logging of it
	g prodfiles 'strings\.(Contains|HasPrefix|HasSuffix|EqualFold|Index)\([[:space:]]*[A-Za-z_.()]*\.Error\(\)|\.Error\(\)[[:space:]]*(==|!=)' |
		grep -iE 'duplicate|unique|violat|foreign key|deadlock|constraint|serializ|lock wait|database is locked|not null|1062|23505'
}

# column_args METHOD KIND — calls .METHOD("...") whose string arguments are not
# what the method takes. KIND: select (column, "col AS alias", "*"), order
# (column with an optional ASC/DESC), group (column). Anything with "(" is
# used as is by Relica and passes. Calls with nested parentheses are skipped.
column_args() {
	g relicafiles "(^|[^A-Za-z0-9_])$1\\(\"" | awk -v m="$1" -v kind="$2" '
		function ok(a,   id, ref, low) {
			if (index(a, "(") > 0 || a == "...") return 1
			id = "[A-Za-z_][A-Za-z0-9_$]*"
			ref = id "(\\." id ")*"
			if (a ~ ("^" ref "(\\.\\*)?$")) return 1
			if (kind == "select") {
				if (a == "*") return 1
				return a ~ ("^" ref "[ \t]+[Aa][Ss][ \t]+" id "$")
			}
			if (kind == "order") {
				low = tolower(a)
				return low ~ ("^" tolower(ref) "[ \t]+(asc|desc)$")
			}
			return 0
		}
		{
			code = $0
			sub(/^[^:]+:[0-9]+:/, "", code)
			s = code
			bad = 0
			while (!bad && match(s, "(^|[^A-Za-z0-9_])" m "\\([^()]*\\)")) {
				call = substr(s, RSTART, RLENGTH)
				s = substr(s, RSTART + RLENGTH)
				while (match(call, /"[^"]*"/)) {
					a = substr(call, RSTART + 1, RLENGTH - 2)
					call = substr(call, RSTART + RLENGTH)
					if (!ok(a)) { bad = 1; break }
				}
			}
			if (bad) print $0
		}'
}

# UPDATE/DELETE builder chains (Update("t") / Delete("t")) that reach
# .Execute() with no Where/AllRows in between, across up to 15 lines. The
# chain is one expression: it ends at a ";" or an unbalanced closing bracket,
# or at a line that closes all its brackets without a trailing "." (Go puts
# the dot at the end of the line). So Model(u).Update("col") — the Model API,
# keyed by the primary key — never borrows a later statement's Execute().
whole_table() {
	awk_files relicafiles '
		# scan(s): walks the code of s, outside strings and // comments, keeping
		# the bracket depth; sets cut (length of the code part of s) and ended
		# (1 if the expression ends inside s).
		function scan(s,   i, n, c, instr) {
			n = length(s); instr = 0; ended = 0; cut = n
			for (i = 1; i <= n; i++) {
				c = substr(s, i, 1)
				if (instr) {
					if (c == "\\") i++
					else if (c == "\"") instr = 0
					continue
				}
				if (c == "\"") instr = 1
				else if (c == "/" && substr(s, i + 1, 1) == "/") { cut = i - 1; return }
				else if (c == "(" || c == "[" || c == "{") depth++
				else if (c == ")" || c == "]" || c == "}") {
					if (--depth < 0) { cut = i; ended = 1; return }
				} else if (c == ";" && depth == 0) { cut = i; ended = 1; return }
			}
		}
		FNR == 1 { open = 0 }
		{
			text = $0
			if (!open && match($0, /(^|[^A-Za-z0-9_])(Update|Delete)\("[^"]*"\)/)) {
				open = 1; start = FNR; code = $0; seen = 0; depth = 0
				text = substr($0, RSTART)
			}
			if (open) {
				scan(text)
				part = substr(text, 1, cut)
				if (part ~ /(^|[^A-Za-z0-9_])(Where|AndWhere|OrWhere|AllRows)\(/) seen = 1
				if (part ~ /(^|[^A-Za-z0-9_])Execute\(\)/) {
					if (!seen) print FILENAME ":" start ":" code
					open = 0
				} else if (ended) open = 0
				else {
					sub(/[ \t\r]+$/, "", part)
					if (depth == 0 && part !~ /\.$/) open = 0
					else if (FNR - start > 15) open = 0
				}
			}
		}'
}

# Builder Upsert("t", …) / OnConflict(…) with no DoUpdate/DoUpdateAll/DoNothing
# on the same line or the next 8 (a chain, or an if/else that picks one).
upsert_without_action() {
	# POSIX awk (mawk on Ubuntu has no ENDFILE): a new file flushes the open hit.
	awk_files relicafiles '
		FNR == 1 { if (open) print hit; open = 0 }
		{
			if (open && $0 ~ /(DoUpdate|DoUpdateAll|DoNothing)\(/) open = 0
			if (open && FNR - start > 8) { print hit; open = 0 }
			if (!open && $0 ~ /(^|[^A-Za-z0-9_])(Upsert\("|OnConflict\()/ && $0 !~ /func / &&
			    $0 !~ /(DoUpdate|DoUpdateAll|DoNothing)\(/) {
				open = 1; start = FNR; hit = FILENAME ":" FNR ":" $0
			}
		}
		END { if (open) print hit }'
}

# Structs that map rows (some field has a db tag) with exported fields that
# have none, or with embedded structs: since v0.20.0 untagged fields use the
# naming rule (CreatedAt → created_at; before: the field name) and embedded
# structs are flattened into columns.
untagged_fields() {
	awk_files relicafiles '
		FNR == 1 { in_s = 0 }
		/^type [A-Za-z0-9_]+ struct[[:space:]]*\{[[:space:]]*$/ { in_s = 1; tagged = 0; n = 0; next }
		in_s && /^\}/ {
			if (tagged) for (i = 1; i <= n; i++) print cand[i]
			in_s = 0; next
		}
		in_s {
			if ($0 ~ /db:"/) { tagged = 1; next }
			line = $0
			sub(/\/\/.*/, "", line)
			if (line ~ /^[[:space:]]*[A-Z][A-Za-z0-9_]*[[:space:]]+[^[:space:]]/ ||
			    line ~ /^[[:space:]]*\*?[A-Z][A-Za-z0-9_.]*[[:space:]]*(`[^`]*`)?[[:space:]]*$/) {
				cand[++n] = FILENAME ":" FNR ":" $0
			}
		}'
}

# Types with db tags and no TableName method, when the repo uses Model():
# since v0.20.0 the derived table is snake_case singular (UserProfile →
# user_profile; before: userprofiles).
models_without_tablename() {
	[ -n "$(g relicafiles '\.Model\(')" ] || return 0
	local named
	# Receiver forms: (T), (*T), (t T), (t *T).
	named=$(g gofiles 'func \(([a-zA-Z_][A-Za-z0-9_]* +)?\*?[A-Za-z0-9_]+\) TableName\(\) string' |
		sed -E 's/.*func \(([a-zA-Z_][A-Za-z0-9_]* +)?\*?([A-Za-z0-9_]+)\) TableName.*/\2/' | sort -u | paste -sd'|')
	awk_files relicafiles '
		FNR == 1 { in_s = 0 }
		/^type [A-Za-z0-9_]+ struct[[:space:]]*\{/ { in_s = 1; name = $2; where = FILENAME ":" FNR ":" $0; next }
		in_s && /^\}/ { in_s = 0; next }
		in_s && /db:"/ && !(name in done) { done[name] = 1; print name "\t" where }' |
		awk -F'\t' -v named="$named" 'BEGIN { split(named, a, "|"); for (i in a) has[a[i]] = 1 } !($1 in has) { print $2 }'
}

# Import names of the relica package: "relica" plus any alias (rdb "github.com/coregx/relica").
relica_names=$(
	{
		echo relica
		g prodfiles '^[[:space:]]*(import[[:space:]]+)?[A-Za-z_][A-Za-z0-9_]*[[:space:]]+"github\.com/coregx/relica"' |
			sed -E 's/^[^:]+:[0-9]+:[[:space:]]*(import[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*).*/\2/'
	} | grep -vx '_' | sort -u | paste -sd'|'
)

# report runs in this shell (process substitution), so its counters survive.
report HIGH H1 'dbcontext With(ctx) returns *relica.DB — it cannot return a tx kept in ctx (check Transactional); before v0.18.0 it also drops ctx' \
	'AGENTS.md › Transactions › Executor' \
	< <(g prodfiles "\\bWith\\([^)]*context\\.Context\\)[[:space:]]*\\*?($relica_names)\\.DB\\b")
report HIGH H2 'Arithmetic as a string value in Set — binds the text, not the expression' \
	'AGENTS.md › Transactions › Rules (relica.NewExp)' \
	< <(g prodfiles '"[A-Za-z_.]+"[[:space:]]*:[[:space:]]*"[A-Za-z_."]+[[:space:]]*[-+*/][[:space:]]*\?"')
report HIGH H3 'defer tx.Rollback() right after Begin, before checking its error — nil-pointer panic when Begin fails' \
	'AGENTS.md › Transactions › Rules' \
	< <(rollback_before_err)
report MEDIUM M1 'DB error classified by message text — breaks on localized servers and MariaDB; use relica.Is* (codes since v0.19.1)' \
	'AGENTS.md › Error Handling' \
	< <(error_text)
report MEDIUM M2 'Hand-written ILIKE — PostgreSQL only, own escaping; relica.ILike (v0.19.1+) works on every database' \
	'AGENTS.md › String Operators' \
	< <(g prodfiles '\bILIKE\b')
report MEDIUM M3 'Assignment to LikeExp.Like — does not compile since v0.19.0; use relica.ILike / NotLike' \
	'docs/MIGRATION_GUIDE.md v0.19.0 → v0.19.1' \
	< <(g gofiles '\.Like[[:space:]]*=[[:space:]]*"')
report MEDIUM M4 'Transaction on context.Background/TODO — request cancellation and deadlines do not reach SQL' \
	'AGENTS.md › Transactions › Rules' \
	< <(g servicefiles '(Transactional(Tx)?|Begin(Tx)?)\([[:space:]]*context\.(Background|TODO)\(\)')
report MEDIUM M5 'SQL built with fmt.Sprintf — confirm every %s/%v is a constant identifier, never input; prefer the builder or {{table}}' \
	'AGENTS.md › Anti-Pattern 2' \
	< <(g prodfiles 'fmt\.Sprintf\("[^"]*\b(SELECT|INSERT INTO|UPDATE|DELETE FROM|WHERE|ORDER BY)\b')
report MEDIUM M6 'LIKE with escaping turned off (.EscapeChars() with no arguments) — % and _ in the value act as wildcards ("_test.go" also matches "latest.go"); a bug unless the value is a pattern on purpose' \
	'AGENTS.md › String Operators' \
	< <(g prodfiles '\.EscapeChars\([[:space:]]*\)')
report MEDIUM M7 'SQLite mode=ro on a plain path — both drivers drop the query, the connection is read-write and creates a missing file; use file:<path>?mode=ro' \
	'AGENTS.md › SQLite' \
	< <(g prodfiles 'mode=ro' | grep -v 'file:' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//')

if [ "$below020" = 1 ]; then
	echo "== Upgrading to v0.20.0: code whose behaviour changes (docs/MIGRATION_GUIDE.md › v0.19.x → v0.20.0)"
else
	echo "== v0.20.0 rules (docs/MIGRATION_GUIDE.md › v0.19.x → v0.20.0)"
fi
echo
report HIGH V1 'OrderBy argument that is not "column [ASC|DESC]" — an error since v0.20.0 (before: everything after the first word was dropped silently); use OrderByExpr' \
	'AGENTS.md › Ordering with Expressions; MIGRATION_GUIDE › OrderBy' \
	< <(column_args OrderBy order)
report HIGH V2 'Literal or expression in Select/AndSelect — quoted as a column name (PostgreSQL/MySQL reject it; SQLite before v0.20.0 returned the text); use SelectExpr' \
	'AGENTS.md › Ordering with Expressions (Select takes column names)' \
	< <(
		column_args Select select
		column_args AndSelect select
	)
report HIGH V3 'Expression in GroupBy — quoted as a column name; use GroupByExpr' \
	'AGENTS.md › Ordering with Expressions' \
	< <(column_args GroupBy group)
report HIGH V4 'UPDATE/DELETE chain reaching Execute() without Where — an error since v0.20.0 (use AllRows() for the whole table on purpose)' \
	'AGENTS.md › UPDATE / DELETE need Where' \
	< <(whole_table)
# Query.QueryParams (removed too, use Params) is left to the compiler: a grep
# cannot tell Relica's Query from echo.Context, whose QueryParams() is common.
report HIGH V5 'Removed in v0.20.0 — does not compile: relica.NewDB (use Open); the compiler also reports Query.QueryParams (use Params)' \
	'MIGRATION_GUIDE › Removed' \
	< <(g gofiles "\\b($relica_names)\\.NewDB\\(")
report MEDIUM V6 'Builder Upsert / OnConflict — needs exactly one of DoUpdate, DoUpdateAll, DoNothing since v0.20.0 (a missing action is an error; check the whole chain)' \
	'AGENTS.md › Upsert; MIGRATION_GUIDE › Upsert' \
	< <(upsert_without_action)
report MEDIUM V7 'Row lock — must run on a transaction since v0.20.0, and never with Union/GroupBy/Distinct/Count/Exists' \
	'AGENTS.md › Row Locking' \
	< <(g relicafiles '(^|[^A-Za-z0-9_])(ForUpdate|ForShare|ForUpdateSkipLocked)\(\)' | grep -v 'func ')
report INFO V8 'Struct with db tags and untagged exported or embedded fields — since v0.20.0 untagged fields map to snake_case columns (CreatedAt → created_at) and embedded structs are flattened; tag them to keep the old names' \
	'AGENTS.md › Table and Column Names' \
	< <(untagged_fields)
report INFO V9 'Row type with db tags and no TableName(), in a repo that uses Model() — matters only for types passed to Model() without a table: since v0.20.0 the derived table is snake_case singular (UserProfile → user_profile; before: userprofiles)' \
	'AGENTS.md › Table and Column Names' \
	< <(models_without_tablename)
report MEDIUM V10 'Count()/Exists() with GroupBy, Distinct, Having or a set operation — since v0.20.0 they follow what All returns (GroupBy: number of groups)' \
	'AGENTS.md › Query Helpers › Exists / Count' \
	< <(g relicafiles '\.(GroupBy|Distinct|Having|Union|UnionAll|Intersect|Except)\(.*\.(Count|Exists)\(\)')
report INFO I5 'OrderBy with a value computed at run time (a sort parameter?) — since v0.20.0 it must be "column [ASC|DESC]"; validate user input against a list of columns' \
	'AGENTS.md › Ordering with Expressions' \
	< <(g relicafiles '(^|[^A-Za-z0-9_])OrderBy\([^")]' | grep -v 'func ')
report INFO I6 'UpdateStruct — since v0.20.0 the primary key is never written into SET' \
	'MIGRATION_GUIDE › Table names, column names, struct scanning' \
	< <(g relicafiles '(^|[^A-Za-z0-9_])UpdateStruct\(' | grep -v 'func ')
if g gomods 'modernc\.org/sqlite|mattn/go-sqlite3' >/dev/null && [ ${#testfiles[@]} -gt 0 ]; then
	report INFO I7 'Test compares generated SQL text and the repo uses SQLite — since v0.20.0 SQLite identifiers are quoted with backticks' \
		'MIGRATION_GUIDE › SQLite: identifiers in backticks' \
		< <(g testfiles 'ToSQL\(\)|\.SQL\(\)')
fi
report INFO I8 'QueryHook / WithLogger — since v0.20.0 they also see raw ExecContext/QueryContext/QueryRowContext and statements rejected at prepare time' \
	'CHANGELOG › v0.20.0 Fixed' \
	< <(g prodfiles '\b(WithQueryHook|WithLogger)\(')
report INFO I1 'Select("*") — Select() already means SELECT *' 'AGENTS.md › API Priority' \
	< <(g relicafiles 'Select\("\*"\)')
report INFO I2 'db.Builder().X — call db.X directly' 'AGENTS.md › API Priority' \
	< <(g relicafiles '\.Builder\(\)\.')
report INFO I3 'Insert with a map — prefer db.Model(&row).Insert()' 'AGENTS.md › Anti-Pattern 1' \
	< <(g relicafiles '\.Insert\("[^"]+",[[:space:]]*map\[string\]')
report INFO I4 'relica.WrapDB — fine at startup; a finding only if it runs per request or per transaction' \
	'AGENTS.md › Transactions › Mistakes found in real projects' \
	< <(g prodfiles 'relica\.WrapDB\(')

cat <<'EOF'
== Not greppable — check by reading the code
  - struct reads (One/All into a struct) whose query does not return a column for every field fail since
    v0.20.0 (ScanRequireFields): list the columns, tag the field db:"-", or use ScanLenient
  - Update/Delete conditions assembled from optional filters (relica.And(conds...) with no conds) render
    empty and are an error since v0.20.0
  - ForUpdate/ForShare/ForUpdateSkipLocked run on tx or With(ctx) inside Transactional
  - no Begin/Transactional nested inside another transaction (join it)
  - no HTTP calls, e-mails or publishing inside a transaction or a retried function
  - retries wrap the whole transaction, only on IsDeadlock || IsSerializationFailure
  - several locked rows are locked in a fixed order (e.g. ascending id)
  - a test proves that a rollback discards writes made through the data layer

EOF
echo "== Summary: HIGH=$high MEDIUM=$medium INFO=$info (grep leads; confirm each in the code)"
exit 0
