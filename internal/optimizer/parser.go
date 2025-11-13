package optimizer

import (
	"fmt"
	"regexp"
	"strings"
)

// WhereClause represents a parsed WHERE clause with its conditions and logic.
type WhereClause struct {
	Conditions []Condition
	Logic      LogicType
}

// Condition represents a single WHERE condition.
type Condition struct {
	Column   string
	Operator string
	Function string // Function name if column is wrapped in function (e.g., "UPPER")
}

// LogicType represents the logical operator between conditions.
type LogicType string

const (
	// LogicAND represents AND logic between conditions
	LogicAND LogicType = "AND"

	// LogicOR represents OR logic between conditions
	LogicOR LogicType = "OR"
)

// ParseWhereClause extracts conditions from a SQL WHERE clause.
// It detects columns, operators, functions, and AND/OR logic.
//
// Example inputs:
//   - "WHERE status = ?"
//   - "WHERE status = ? AND email = ?"
//   - "WHERE status = ? OR name = ?"
//   - "WHERE UPPER(email) = ?"
//
// The parser is simplified and focuses on common patterns.
// Complex nested logic may not be fully parsed.
func ParseWhereClause(sql string) (*WhereClause, error) {
	sql = strings.ToLower(strings.TrimSpace(sql))

	// Extract WHERE clause
	whereClause := extractWhereClauseText(sql)
	if whereClause == "" {
		return &WhereClause{
			Conditions: []Condition{},
			Logic:      LogicAND,
		}, nil
	}

	// Detect logic type (AND or OR)
	// Default to AND, switch to OR if OR is more dominant
	logic := detectLogicType(whereClause)

	// Extract conditions
	conditions := parseConditions(whereClause)

	return &WhereClause{
		Conditions: conditions,
		Logic:      logic,
	}, nil
}

// extractWhereClauseText extracts the WHERE clause text from SQL.
// It handles cases where WHERE is already present or missing.
func extractWhereClauseText(sql string) string {
	// If already starts with "where ", strip it and return the rest
	if strings.HasPrefix(sql, "where ") {
		whereClause := sql[6:]
		// Remove terminators
		for _, terminator := range []string{" order by", " group by", " limit", " having", ";"} {
			if idx := strings.Index(whereClause, terminator); idx != -1 {
				whereClause = whereClause[:idx]
			}
		}
		return strings.TrimSpace(whereClause)
	}

	// Find WHERE keyword in the middle
	whereIdx := strings.Index(sql, " where ")
	if whereIdx == -1 {
		return ""
	}

	// Extract everything after WHERE
	whereClause := sql[whereIdx+7:] // +7 for " where "

	// Remove terminators (ORDER BY, GROUP BY, LIMIT, HAVING, ;)
	for _, terminator := range []string{" order by", " group by", " limit", " having", ";"} {
		if idx := strings.Index(whereClause, terminator); idx != -1 {
			whereClause = whereClause[:idx]
		}
	}

	return strings.TrimSpace(whereClause)
}

// detectLogicType determines if the WHERE clause uses AND or OR logic.
// This is a heuristic: if OR appears more than AND, it's OR logic.
func detectLogicType(whereClause string) LogicType {
	andCount := strings.Count(whereClause, " and ")
	orCount := strings.Count(whereClause, " or ")

	if orCount > andCount {
		return LogicOR
	}
	return LogicAND
}

// parseConditions extracts individual conditions from WHERE clause.
func parseConditions(whereClause string) []Condition {
	// Pre-allocate slice (estimate: 1-5 conditions typically)
	conditions := make([]Condition, 0, 5)

	// Split by AND and OR (we'll process them together)
	// Regex: match column [function] operator [value]
	// Supports: col = ?, col > ?, UPPER(col) = ?, col IN (?), col LIKE ?

	// First, try to match function calls: FUNC(column) operator
	funcPattern := `([a-z_][a-z0-9_]*)\s*\(\s*([a-z_][a-z0-9_]*)\s*\)\s*(=|!=|<>|>|<|>=|<=|like|in|not\s+in|between)`
	funcRe := regexp.MustCompile(funcPattern)

	funcMatches := funcRe.FindAllStringSubmatch(whereClause, -1)
	for _, match := range funcMatches {
		if len(match) < 4 {
			continue
		}

		function := match[1] // Function name (e.g., "upper")
		column := match[2]   // Column name
		operator := match[3] // Operator

		// Skip SQL keywords
		if isSQLKeyword(column) {
			continue
		}

		// Normalize operator
		operator = normalizeOperator(operator)

		conditions = append(conditions, Condition{
			Column:   column,
			Operator: operator,
			Function: strings.ToUpper(function),
		})
	}

	// Remove function calls from whereClause to avoid double-matching
	whereClauseNoFunc := funcRe.ReplaceAllString(whereClause, "")

	// Match simple column operator patterns (no functions)
	// Pattern needs to match: column operator [value|placeholder|etc]
	// We don't need to match the value, just column and operator
	simplePattern := `([a-z_][a-z0-9_]*)\s*(=|!=|<>|>=|<=|>|<|like|in|not\s+in|between)`
	simpleRe := regexp.MustCompile(simplePattern)

	simpleMatches := simpleRe.FindAllStringSubmatch(whereClauseNoFunc, -1)
	for _, match := range simpleMatches {
		if len(match) < 3 {
			continue
		}

		column := match[1]   // Column name
		operator := match[2] // Operator

		// Skip SQL keywords
		if isSQLKeyword(column) {
			continue
		}

		// Normalize operator
		operator = normalizeOperator(operator)

		conditions = append(conditions, Condition{
			Column:   column,
			Operator: operator,
			Function: "",
		})
	}

	return conditions
}

// normalizeOperator standardizes operator representation.
func normalizeOperator(op string) string {
	op = strings.TrimSpace(strings.ToUpper(op))
	switch op {
	case "<>":
		return "!="
	case "NOT IN":
		return "NOT_IN"
	default:
		return op
	}
}

// extractJoinColumns extracts the two columns from a JOIN ON clause.
// Example: "JOIN orders ON users.id = orders.user_id"
// Returns: ("users.id", "orders.user_id")
// Example: "LEFT JOIN orders o ON u.id = o.user_id"
// Returns: ("u.id", "o.user_id")
//
// For aliased tables, we need to map alias to table name.
// This function returns the actual table name on the right side.
func extractJoinColumns(join string) (leftCol, rightCol string) {
	join = strings.ToLower(strings.TrimSpace(join))

	// Pattern: [JOIN_TYPE] JOIN table [AS alias] ON col1 = col2
	// Make "as" optional and allow short aliases
	pattern := `(?:inner|left|right|full)?\s*join\s+([a-z_][a-z0-9_]*)(?:\s+(?:as\s+)?([a-z_][a-z0-9_]*))?\s+on\s+([a-z_][a-z0-9_.]+)\s*=\s*([a-z_][a-z0-9_.]+)`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(join)
	if len(matches) >= 5 {
		tableName := matches[1] // Real table name
		alias := matches[2]     // Alias (may be empty)
		leftCol = matches[3]    // Left column
		rightCol = matches[4]   // Right column

		// If rightCol starts with alias, replace alias with table name
		if alias != "" {
			rightPrefix := alias + "."
			if strings.HasPrefix(rightCol, rightPrefix) {
				rightCol = tableName + "." + strings.TrimPrefix(rightCol, rightPrefix)
			}
		}

		return leftCol, rightCol
	}

	return "", ""
}

// extractTableName extracts table name from a qualified column reference.
// Example: "users.id" -> "users"
// Example: "id" -> ""
func extractTableNameFromColumn(column string) string {
	parts := strings.Split(column, ".")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

// extractColumnName extracts column name from a qualified reference.
// Example: "users.id" -> "id"
// Example: "id" -> "id"
// Example: "schema.users.id" -> "id" (last part)
func extractColumnName(column string) string {
	parts := strings.Split(column, ".")
	if len(parts) == 0 {
		return column
	}
	// Return the last part (column name)
	return parts[len(parts)-1]
}

// extractJoinClauses extracts all JOIN clauses from a SQL query.
func extractJoinClauses(sql string) []string {
	sql = strings.ToLower(sql)
	joinSection := extractJoinSection(sql)
	return findAllJoins(joinSection)
}

// extractJoinSection extracts the section containing JOINs (before WHERE/ORDER BY/etc).
func extractJoinSection(sql string) string {
	keywords := []string{" where ", " order by ", " group by ", " limit ", " having "}
	endPos := len(sql)
	for _, kw := range keywords {
		if pos := strings.Index(sql, kw); pos != -1 && pos < endPos {
			endPos = pos
		}
	}
	return sql[:endPos]
}

// findAllJoins finds all JOIN clauses in the given SQL section.
func findAllJoins(joinSection string) []string {
	var joins []string
	joinKeyword := "join"
	startPos := 0

	for {
		joinPos := strings.Index(joinSection[startPos:], joinKeyword)
		if joinPos == -1 {
			break
		}
		joinPos += startPos

		joinStart := findJoinStart(joinSection, joinPos)
		joinEnd := findJoinEnd(joinSection, joinPos)

		joinClause := strings.TrimSpace(joinSection[joinStart:joinEnd])
		if joinClause != "" {
			joins = append(joins, joinClause)
		}

		startPos = joinPos + 4
	}

	return joins
}

// findJoinStart finds the start position of a JOIN clause (including type prefix).
func findJoinStart(sql string, joinPos int) int {
	if joinPos < 10 {
		return joinPos
	}

	prefix := sql[joinPos-10 : joinPos]
	for _, jt := range []string{"inner ", "left ", "right ", "full ", "cross "} {
		if strings.Contains(prefix, jt) {
			return joinPos - len(jt)
		}
	}
	return joinPos
}

// findJoinEnd finds the end position of a JOIN clause (before next JOIN).
func findJoinEnd(sql string, joinPos int) int {
	nextJoinPos := strings.Index(sql[joinPos+4:], "join")
	if nextJoinPos == -1 {
		return len(sql)
	}

	joinEnd := joinPos + 4 + nextJoinPos
	// Back up over trailing spaces
	for joinEnd > joinPos && sql[joinEnd-1] == ' ' {
		joinEnd--
	}
	return joinEnd
}

// extractSelectColumns extracts column names from SELECT clause.
// This is simplified and may not handle all cases (e.g., complex expressions).
func extractSelectColumns(sql string) []string {
	sql = strings.ToLower(sql)

	// Find SELECT clause
	selectIdx := strings.Index(sql, "select ")
	if selectIdx == -1 {
		return nil
	}

	// Extract until FROM
	fromIdx := strings.Index(sql[selectIdx:], " from ")
	if fromIdx == -1 {
		return nil
	}

	selectClause := sql[selectIdx+7 : selectIdx+fromIdx]

	// Handle SELECT *
	if strings.TrimSpace(selectClause) == "*" {
		return []string{"*"}
	}

	// Split by comma and extract column names
	var columns []string
	parts := strings.Split(selectClause, ",")
	columnPattern := regexp.MustCompile(`([a-z_][a-z0-9_.]*)\s*(?:as\s+[a-z_][a-z0-9_]*)?`)

	for _, part := range parts {
		matches := columnPattern.FindStringSubmatch(strings.TrimSpace(part))
		if len(matches) >= 2 {
			col := extractColumnName(matches[1])
			if !isSQLKeyword(col) && col != "*" {
				columns = append(columns, col)
			}
		}
	}

	return columns
}

// ParseError represents an error during SQL parsing.
type ParseError struct {
	Message string
	SQL     string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error: %s (sql: %s)", e.Message, e.SQL)
}
