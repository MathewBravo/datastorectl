package parse

import (
	"fmt"
	"sort"
	"strings"
)

// GrantStmt is the parsed shape of a single GRANT DDL statement from
// SHOW GRANTS output. Identity is the (User, Host, Database, Table)
// tuple. Privileges are uppercased and sorted for deterministic diffs.
type GrantStmt struct {
	Privileges  []string // sorted, uppercased; "ALL" stands alone
	Database    string   // "*" for global scope, or schema name
	Table       string   // "*" for schema-level, or table name
	User        string
	Host        string
	GrantOption bool
}

// ParseGrant parses a single GRANT statement. The version argument is
// reserved for version-specific tolerance knobs; currently unused
// (MySQL 8.0 and 8.4 emit identical GRANT syntax for the scopes we
// support).
func ParseGrant(ddl, version string) (GrantStmt, error) {
	_ = version
	p := &grantParser{lex: newLexer(ddl)}
	return p.parse()
}

// ParseGrantLines splits the output of SHOW GRANTS FOR ... on newlines
// and parses each line as an individual GRANT statement. Blank lines
// and leading/trailing whitespace are tolerated. A parse error on any
// line aborts and returns the error.
func ParseGrantLines(output, version string) ([]GrantStmt, error) {
	var stmts []GrantStmt
	for i, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		stmt, err := ParseGrant(trimmed, version)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		stmts = append(stmts, stmt)
	}
	return stmts, nil
}

type grantParser struct {
	lex    *lexer
	peeked *token
}

func (p *grantParser) next() (token, error) {
	if p.peeked != nil {
		t := *p.peeked
		p.peeked = nil
		return t, nil
	}
	return p.lex.next()
}

func (p *grantParser) peek() (token, error) {
	if p.peeked != nil {
		return *p.peeked, nil
	}
	t, err := p.lex.next()
	if err != nil {
		return token{}, err
	}
	p.peeked = &t
	return t, nil
}

func (p *grantParser) expectKeyword(name string) error {
	t, err := p.next()
	if err != nil {
		return err
	}
	if !matchIdent(t, name) {
		return fmt.Errorf("parse: expected %q, got %s", name, t)
	}
	return nil
}

func (p *grantParser) parse() (GrantStmt, error) {
	if err := p.expectKeyword("GRANT"); err != nil {
		return GrantStmt{}, err
	}

	privs, err := p.readPrivileges()
	if err != nil {
		return GrantStmt{}, err
	}

	if err := p.expectKeyword("ON"); err != nil {
		return GrantStmt{}, err
	}

	// Reject routine scopes early with a specific diagnostic.
	t, err := p.peek()
	if err != nil {
		return GrantStmt{}, err
	}
	if matchIdent(t, "PROCEDURE") || matchIdent(t, "FUNCTION") {
		return GrantStmt{}, fmt.Errorf("parse: %s-scope grants are not supported in this release", strings.ToUpper(t.Value))
	}

	db, tbl, err := p.readScope()
	if err != nil {
		return GrantStmt{}, err
	}

	if err := p.expectKeyword("TO"); err != nil {
		return GrantStmt{}, err
	}

	user, err := p.readQuotedName()
	if err != nil {
		return GrantStmt{}, err
	}
	at, err := p.next()
	if err != nil {
		return GrantStmt{}, err
	}
	if at.Kind != tkAt {
		return GrantStmt{}, fmt.Errorf("parse: expected '@' between user and host, got %s", at)
	}
	host, err := p.readQuotedName()
	if err != nil {
		return GrantStmt{}, err
	}

	stmt := GrantStmt{
		Privileges:  privs,
		Database:    db,
		Table:       tbl,
		User:        user,
		Host:        host,
		GrantOption: false,
	}

	// Optional WITH GRANT OPTION trailer.
	t, err = p.peek()
	if err != nil {
		return GrantStmt{}, err
	}
	if matchIdent(t, "WITH") {
		_, _ = p.next()
		if err := p.expectKeyword("GRANT"); err != nil {
			return GrantStmt{}, fmt.Errorf("parse: WITH must be followed by GRANT OPTION: %w", err)
		}
		if err := p.expectKeyword("OPTION"); err != nil {
			return GrantStmt{}, err
		}
		stmt.GrantOption = true
	}

	// Tolerate a trailing semicolon.
	t, err = p.peek()
	if err == nil && t.Kind == tkSemi {
		_, _ = p.next()
	}
	// Anything else trailing is an error — surfaces drift in server output.
	t, err = p.peek()
	if err != nil {
		return GrantStmt{}, err
	}
	if t.Kind != tkEOF {
		return GrantStmt{}, fmt.Errorf("parse: unexpected trailing token %s", t)
	}

	return stmt, nil
}

// readPrivileges consumes the comma-separated privilege list up to
// (but not including) the ON keyword. Privilege names can be
// multi-word (SHOW DATABASES, REPLICATION CLIENT, CREATE TEMPORARY
// TABLES), so we accumulate words until we hit a comma or ON. A '('
// after a privilege name signals a column-level grant — rejected.
//
// Returns a sorted, uppercased slice with "ALL PRIVILEGES" normalized
// to "ALL" for deterministic diffs.
func (p *grantParser) readPrivileges() ([]string, error) {
	var privs []string
	for {
		name, err := p.readOnePrivilege()
		if err != nil {
			return nil, err
		}
		privs = append(privs, normalizePrivilege(name))
		t, err := p.peek()
		if err != nil {
			return nil, err
		}
		if t.Kind == tkComma {
			_, _ = p.next()
			continue
		}
		// Expect ON next.
		if matchIdent(t, "ON") {
			break
		}
		return nil, fmt.Errorf("parse: expected ',' or ON after privilege list, got %s", t)
	}
	sort.Strings(privs)
	return dedupe(privs), nil
}

// readOnePrivilege consumes one privilege name, which may span multiple
// idents (e.g. "SHOW DATABASES"). Stops at ',', ON, or '(' (column-level).
func (p *grantParser) readOnePrivilege() (string, error) {
	var words []string
	for {
		t, err := p.peek()
		if err != nil {
			return "", err
		}
		if t.Kind != tkIdent {
			if len(words) == 0 {
				return "", fmt.Errorf("parse: expected privilege name, got %s", t)
			}
			return strings.Join(words, " "), nil
		}
		if matchIdent(t, "ON") && len(words) > 0 {
			return strings.Join(words, " "), nil
		}
		_, _ = p.next()
		words = append(words, strings.ToUpper(t.Value))
		// Check for column-list opener after this word.
		next, err := p.peek()
		if err != nil {
			return "", err
		}
		if next.Kind == tkLParen {
			return "", fmt.Errorf("parse: column-level grants are not supported in this release")
		}
		if next.Kind == tkComma || matchIdent(next, "ON") {
			return strings.Join(words, " "), nil
		}
	}
}

// normalizePrivilege collapses "ALL PRIVILEGES" to "ALL" and
// uppercases everything.
func normalizePrivilege(name string) string {
	up := strings.ToUpper(strings.TrimSpace(name))
	if up == "ALL PRIVILEGES" {
		return "ALL"
	}
	return up
}

// dedupe returns the slice with consecutive duplicates removed.
// The input must already be sorted.
func dedupe(s []string) []string {
	if len(s) < 2 {
		return s
	}
	out := s[:1]
	for i := 1; i < len(s); i++ {
		if s[i] != out[len(out)-1] {
			out = append(out, s[i])
		}
	}
	return out
}

// readScope consumes the ON <db>.<table> or ON *.* portion. Returns
// (database, table) where table == "*" for schema scope and
// database == table == "*" for global scope.
func (p *grantParser) readScope() (string, string, error) {
	// Read database half (before the dot).
	t, err := p.next()
	if err != nil {
		return "", "", err
	}
	var db string
	switch t.Kind {
	case tkStar:
		db = "*"
	case tkBacktick, tkString:
		db = t.Value
	case tkIdent:
		db = t.Value
	default:
		return "", "", fmt.Errorf("parse: expected schema name or '*', got %s", t)
	}

	// Expect '.'
	dot, err := p.next()
	if err != nil {
		return "", "", err
	}
	if dot.Kind != tkDot {
		return "", "", fmt.Errorf("parse: expected '.' in scope, got %s", dot)
	}

	// Read table half (after the dot).
	t, err = p.next()
	if err != nil {
		return "", "", err
	}
	var table string
	switch t.Kind {
	case tkStar:
		table = "*"
	case tkBacktick, tkString:
		table = t.Value
	case tkIdent:
		table = t.Value
	default:
		return "", "", fmt.Errorf("parse: expected table name or '*', got %s", t)
	}

	return db, table, nil
}

// readQuotedName accepts a backtick-quoted or single-quoted
// identifier (user or host name).
func (p *grantParser) readQuotedName() (string, error) {
	t, err := p.next()
	if err != nil {
		return "", err
	}
	switch t.Kind {
	case tkBacktick, tkString:
		return t.Value, nil
	}
	return "", fmt.Errorf("parse: expected quoted name, got %s", t)
}
