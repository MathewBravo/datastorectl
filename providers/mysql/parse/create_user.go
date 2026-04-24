package parse

import (
	"fmt"
)

// CreateUserStmt is the parsed shape of a SHOW CREATE USER output.
// Fields map directly onto mysql_user DCL attributes that the handler
// in Phase 23 will consume. Empty strings and zero ints mean "not
// emitted by the server" — MySQL 8.0+ emits every clause with a
// default value, so zero values are rare in practice.
type CreateUserStmt struct {
	User string
	Host string

	// Authentication
	Plugin     string
	AuthString string

	// REQUIRE clause
	RequireNone    bool
	RequireSSL     bool
	RequireX509    bool
	RequireIssuer  string
	RequireSubject string
	RequireCipher  string

	// Resource limits (WITH clause)
	MaxQueriesPerHour     int
	MaxConnectionsPerHour int
	MaxUpdatesPerHour     int
	MaxUserConnections    int

	// Password policy
	PasswordExpire         string // "DEFAULT", "NEVER", "INTERVAL", or empty
	PasswordExpireInterval int    // days, when PasswordExpire == "INTERVAL"
	PasswordHistory        string // "DEFAULT" or "N" (literal digits)
	PasswordHistoryCount   int    // when PasswordHistory == "N"
	PasswordReuse          string // "DEFAULT" or "INTERVAL"
	PasswordReuseInterval  int    // days, when PasswordReuse == "INTERVAL"
	PasswordRequireCurrent string // "DEFAULT", "OPTIONAL", or empty
	FailedLoginAttempts    int
	PasswordLockTime       string // "N" (digits) or "UNBOUNDED" or empty

	// Account state
	AccountLocked bool

	// Metadata
	Comment   string
	Attribute string

	// Default roles set on the user. Populated when SHOW CREATE USER
	// emits a `DEFAULT ROLE <role>[, <role>]*` clause (MySQL 8 WL#988;
	// set for every RDS Aurora `admin` user). Parsed so discover does
	// not error; not currently surfaced as a mysql_user body attribute.
	// omitempty so fixtures for users without default roles don't need
	// regeneration.
	DefaultRoles []DefaultRole `json:",omitempty"`
}

// DefaultRole is a (name, host) pair from a DEFAULT ROLE clause.
type DefaultRole struct {
	Name string
	Host string
}

// ParseCreateUser parses one CREATE USER DDL statement produced by
// SHOW CREATE USER. The version argument selects version-specific
// tolerance knobs (currently a no-op; wiring for future use).
func ParseCreateUser(ddl, version string) (CreateUserStmt, error) {
	_ = version // reserved for future version-specific parsing
	p := &createUserParser{lex: newLexer(ddl)}
	return p.parse()
}

type createUserParser struct {
	lex    *lexer
	peeked *token
}

func (p *createUserParser) next() (token, error) {
	if p.peeked != nil {
		t := *p.peeked
		p.peeked = nil
		return t, nil
	}
	return p.lex.next()
}

func (p *createUserParser) peek() (token, error) {
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

func (p *createUserParser) expectKeyword(name string) error {
	t, err := p.next()
	if err != nil {
		return err
	}
	if !matchIdent(t, name) {
		return fmt.Errorf("parse: expected %q, got %s", name, t)
	}
	return nil
}

func (p *createUserParser) parse() (CreateUserStmt, error) {
	if err := p.expectKeyword("CREATE"); err != nil {
		return CreateUserStmt{}, err
	}
	if err := p.expectKeyword("USER"); err != nil {
		return CreateUserStmt{}, err
	}

	user, err := p.readQuotedOrBacktickIdent()
	if err != nil {
		return CreateUserStmt{}, err
	}
	t, err := p.next()
	if err != nil {
		return CreateUserStmt{}, err
	}
	if t.Kind != tkAt {
		return CreateUserStmt{}, fmt.Errorf("parse: expected '@' between user and host, got %s", t)
	}
	host, err := p.readQuotedOrBacktickIdent()
	if err != nil {
		return CreateUserStmt{}, err
	}

	stmt := CreateUserStmt{User: user, Host: host}

	for {
		t, err := p.peek()
		if err != nil {
			return CreateUserStmt{}, err
		}
		if t.Kind == tkEOF || t.Kind == tkSemi {
			break
		}
		if err := p.parseClause(&stmt); err != nil {
			return CreateUserStmt{}, err
		}
	}
	return stmt, nil
}

// readQuotedOrBacktickIdent accepts either a backticked or
// single-quoted identifier. SHOW CREATE USER uses backticks, but some
// contexts allow the quoted form; both map to the same extracted name.
func (p *createUserParser) readQuotedOrBacktickIdent() (string, error) {
	t, err := p.next()
	if err != nil {
		return "", err
	}
	switch t.Kind {
	case tkBacktick, tkString:
		return t.Value, nil
	}
	return "", fmt.Errorf("parse: expected quoted identifier, got %s", t)
}

// parseClause consumes one top-level CREATE USER clause and updates
// stmt accordingly. Returns an error for unknown clauses so we catch
// new MySQL versions emitting syntax we haven't mapped yet.
func (p *createUserParser) parseClause(stmt *CreateUserStmt) error {
	t, err := p.next()
	if err != nil {
		return err
	}
	if t.Kind != tkIdent {
		return fmt.Errorf("parse: expected clause keyword, got %s", t)
	}
	switch {
	case equalsFoldAny(t.Value, "IDENTIFIED"):
		return p.parseIdentifiedClause(stmt)
	case equalsFoldAny(t.Value, "REQUIRE"):
		return p.parseRequireClause(stmt)
	case equalsFoldAny(t.Value, "WITH"):
		return p.parseWithClause(stmt)
	case equalsFoldAny(t.Value, "PASSWORD"):
		return p.parsePasswordClause(stmt)
	case equalsFoldAny(t.Value, "ACCOUNT"):
		return p.parseAccountClause(stmt)
	case equalsFoldAny(t.Value, "FAILED_LOGIN_ATTEMPTS"):
		n, err := p.readNumber()
		if err != nil {
			return err
		}
		stmt.FailedLoginAttempts = n
		return nil
	case equalsFoldAny(t.Value, "PASSWORD_LOCK_TIME"):
		return p.parsePasswordLockTime(stmt)
	case equalsFoldAny(t.Value, "COMMENT"):
		s, err := p.readString()
		if err != nil {
			return err
		}
		stmt.Comment = s
		return nil
	case equalsFoldAny(t.Value, "ATTRIBUTE"):
		s, err := p.readString()
		if err != nil {
			return err
		}
		stmt.Attribute = s
		return nil
	case equalsFoldAny(t.Value, "DEFAULT"):
		return p.parseDefaultClause(stmt)
	}
	return fmt.Errorf("parse: unknown CREATE USER clause %q", t.Value)
}

// parseIdentifiedClause consumes `IDENTIFIED WITH 'plugin' AS 'hash'`
// or `IDENTIFIED WITH 'plugin'` (no AS, some MySQL variants).
func (p *createUserParser) parseIdentifiedClause(stmt *CreateUserStmt) error {
	if err := p.expectKeyword("WITH"); err != nil {
		return err
	}
	plugin, err := p.readString()
	if err != nil {
		// Some forms use a bare ident (legacy). Fall back to that.
		return fmt.Errorf("parse: IDENTIFIED WITH requires a quoted plugin name: %w", err)
	}
	stmt.Plugin = plugin

	t, err := p.peek()
	if err != nil {
		return err
	}
	if matchIdent(t, "AS") {
		_, _ = p.next()
		auth, err := p.readString()
		if err != nil {
			return err
		}
		stmt.AuthString = auth
	}
	return nil
}

// parseRequireClause consumes REQUIRE NONE or REQUIRE SSL or
// REQUIRE X509 or REQUIRE <field> '...' [<field> '...']...
//
// MySQL's SHOW CREATE USER output separates fields with whitespace
// (no AND keyword) — "REQUIRE SUBJECT '...' ISSUER '...' CIPHER '...'"
// — while the input form uses AND. The parser tolerates both.
func (p *createUserParser) parseRequireClause(stmt *CreateUserStmt) error {
	t, err := p.peek()
	if err != nil {
		return err
	}
	if matchIdent(t, "NONE") {
		_, _ = p.next()
		stmt.RequireNone = true
		return nil
	}
	for {
		t, err := p.peek()
		if err != nil {
			return err
		}
		if t.Kind != tkIdent {
			return nil
		}
		up := t.Value
		switch {
		case equalsFoldAny(up, "SSL"):
			_, _ = p.next()
			stmt.RequireSSL = true
		case equalsFoldAny(up, "X509"):
			_, _ = p.next()
			stmt.RequireX509 = true
		case equalsFoldAny(up, "SUBJECT"):
			_, _ = p.next()
			s, err := p.readString()
			if err != nil {
				return err
			}
			stmt.RequireSubject = s
		case equalsFoldAny(up, "ISSUER"):
			_, _ = p.next()
			s, err := p.readString()
			if err != nil {
				return err
			}
			stmt.RequireIssuer = s
		case equalsFoldAny(up, "CIPHER"):
			_, _ = p.next()
			s, err := p.readString()
			if err != nil {
				return err
			}
			stmt.RequireCipher = s
		case equalsFoldAny(up, "AND"):
			// Tolerate input form's AND separators between fields.
			_, _ = p.next()
			continue
		default:
			return nil // not our keyword — next clause
		}
	}
}

// parseWithClause consumes WITH followed by one or more resource
// limits. Limits are space-separated, each a keyword plus number.
func (p *createUserParser) parseWithClause(stmt *CreateUserStmt) error {
	for {
		t, err := p.peek()
		if err != nil {
			return err
		}
		if t.Kind != tkIdent {
			return nil
		}
		var dst *int
		switch {
		case equalsFoldAny(t.Value, "MAX_QUERIES_PER_HOUR"):
			dst = &stmt.MaxQueriesPerHour
		case equalsFoldAny(t.Value, "MAX_CONNECTIONS_PER_HOUR"):
			dst = &stmt.MaxConnectionsPerHour
		case equalsFoldAny(t.Value, "MAX_UPDATES_PER_HOUR"):
			dst = &stmt.MaxUpdatesPerHour
		case equalsFoldAny(t.Value, "MAX_USER_CONNECTIONS"):
			dst = &stmt.MaxUserConnections
		default:
			return nil // next clause
		}
		_, _ = p.next()
		n, err := p.readNumber()
		if err != nil {
			return err
		}
		*dst = n
	}
}

// parsePasswordClause handles the several PASSWORD ... sub-forms.
func (p *createUserParser) parsePasswordClause(stmt *CreateUserStmt) error {
	t, err := p.next()
	if err != nil {
		return err
	}
	if t.Kind != tkIdent {
		return fmt.Errorf("parse: expected word after PASSWORD, got %s", t)
	}
	switch {
	case equalsFoldAny(t.Value, "EXPIRE"):
		return p.parsePasswordExpire(stmt)
	case equalsFoldAny(t.Value, "HISTORY"):
		return p.parsePasswordHistory(stmt)
	case equalsFoldAny(t.Value, "REUSE"):
		return p.parsePasswordReuse(stmt)
	case equalsFoldAny(t.Value, "REQUIRE"):
		return p.parsePasswordRequireCurrent(stmt)
	}
	return fmt.Errorf("parse: unknown PASSWORD sub-clause %q", t.Value)
}

// parsePasswordExpire handles:
//
//	PASSWORD EXPIRE                     — mark expired now (bare form)
//	PASSWORD EXPIRE DEFAULT
//	PASSWORD EXPIRE NEVER
//	PASSWORD EXPIRE INTERVAL <n> DAY
func (p *createUserParser) parsePasswordExpire(stmt *CreateUserStmt) error {
	t, err := p.peek()
	if err != nil {
		return err
	}
	if t.Kind != tkIdent {
		stmt.PasswordExpire = "EXPIRED"
		return nil
	}
	switch {
	case equalsFoldAny(t.Value, "DEFAULT"):
		_, _ = p.next()
		stmt.PasswordExpire = "DEFAULT"
		return nil
	case equalsFoldAny(t.Value, "NEVER"):
		_, _ = p.next()
		stmt.PasswordExpire = "NEVER"
		return nil
	case equalsFoldAny(t.Value, "INTERVAL"):
		_, _ = p.next()
		stmt.PasswordExpire = "INTERVAL"
		n, err := p.readNumber()
		if err != nil {
			return err
		}
		stmt.PasswordExpireInterval = n
		if err := p.expectKeyword("DAY"); err != nil {
			return err
		}
		return nil
	}
	// Not a recognized modifier — bare form (expire now).
	stmt.PasswordExpire = "EXPIRED"
	return nil
}

// parsePasswordHistory handles:
//
//	PASSWORD HISTORY DEFAULT
//	PASSWORD HISTORY <n>
func (p *createUserParser) parsePasswordHistory(stmt *CreateUserStmt) error {
	t, err := p.peek()
	if err != nil {
		return err
	}
	if matchIdent(t, "DEFAULT") {
		_, _ = p.next()
		stmt.PasswordHistory = "DEFAULT"
		return nil
	}
	n, err := p.readNumber()
	if err != nil {
		return err
	}
	stmt.PasswordHistory = "N"
	stmt.PasswordHistoryCount = n
	return nil
}

// parsePasswordReuse handles:
//
//	PASSWORD REUSE INTERVAL DEFAULT
//	PASSWORD REUSE INTERVAL <n> DAY
func (p *createUserParser) parsePasswordReuse(stmt *CreateUserStmt) error {
	if err := p.expectKeyword("INTERVAL"); err != nil {
		return err
	}
	t, err := p.peek()
	if err != nil {
		return err
	}
	if matchIdent(t, "DEFAULT") {
		_, _ = p.next()
		stmt.PasswordReuse = "DEFAULT"
		return nil
	}
	n, err := p.readNumber()
	if err != nil {
		return err
	}
	stmt.PasswordReuse = "INTERVAL"
	stmt.PasswordReuseInterval = n
	if err := p.expectKeyword("DAY"); err != nil {
		return err
	}
	return nil
}

// parsePasswordRequireCurrent handles:
//
//	PASSWORD REQUIRE CURRENT DEFAULT
//	PASSWORD REQUIRE CURRENT OPTIONAL
//	PASSWORD REQUIRE CURRENT
func (p *createUserParser) parsePasswordRequireCurrent(stmt *CreateUserStmt) error {
	if err := p.expectKeyword("CURRENT"); err != nil {
		return err
	}
	t, err := p.peek()
	if err != nil {
		return err
	}
	if t.Kind != tkIdent {
		stmt.PasswordRequireCurrent = "ENFORCED"
		return nil
	}
	switch {
	case equalsFoldAny(t.Value, "DEFAULT"):
		_, _ = p.next()
		stmt.PasswordRequireCurrent = "DEFAULT"
	case equalsFoldAny(t.Value, "OPTIONAL"):
		_, _ = p.next()
		stmt.PasswordRequireCurrent = "OPTIONAL"
	default:
		// Bare `PASSWORD REQUIRE CURRENT` (no modifier) = enforced.
		stmt.PasswordRequireCurrent = "ENFORCED"
	}
	return nil
}

// parseAccountClause handles ACCOUNT LOCK and ACCOUNT UNLOCK.
func (p *createUserParser) parseAccountClause(stmt *CreateUserStmt) error {
	t, err := p.next()
	if err != nil {
		return err
	}
	switch {
	case matchIdent(t, "LOCK"):
		stmt.AccountLocked = true
	case matchIdent(t, "UNLOCK"):
		stmt.AccountLocked = false
	default:
		return fmt.Errorf("parse: expected LOCK or UNLOCK after ACCOUNT, got %s", t)
	}
	return nil
}

// parseDefaultClause handles DEFAULT ROLE <role>[, <role>]*. Each role
// is a (name, host) pair written `ident`@`ident` or 'ident'@'ident'.
// DEFAULT appears in SHOW CREATE USER output for any user that has
// default roles set (MySQL 8 WL#988); every RDS Aurora admin user has
// them.
func (p *createUserParser) parseDefaultClause(stmt *CreateUserStmt) error {
	if err := p.expectKeyword("ROLE"); err != nil {
		return err
	}
	for {
		role, err := p.readRoleIdent()
		if err != nil {
			return err
		}
		stmt.DefaultRoles = append(stmt.DefaultRoles, role)

		t, err := p.peek()
		if err != nil {
			return err
		}
		if t.Kind != tkComma {
			return nil
		}
		_, _ = p.next() // consume comma, loop for next role
	}
}

// readRoleIdent consumes one `name`@`host` (or 'name'@'host') pair.
func (p *createUserParser) readRoleIdent() (DefaultRole, error) {
	name, err := p.readQuotedOrBacktickIdent()
	if err != nil {
		return DefaultRole{}, err
	}
	t, err := p.next()
	if err != nil {
		return DefaultRole{}, err
	}
	if t.Kind != tkAt {
		return DefaultRole{}, fmt.Errorf("parse: expected '@' in DEFAULT ROLE role spec, got %s", t)
	}
	host, err := p.readQuotedOrBacktickIdent()
	if err != nil {
		return DefaultRole{}, err
	}
	return DefaultRole{Name: name, Host: host}, nil
}

// parsePasswordLockTime handles PASSWORD_LOCK_TIME <N> and
// PASSWORD_LOCK_TIME UNBOUNDED.
func (p *createUserParser) parsePasswordLockTime(stmt *CreateUserStmt) error {
	t, err := p.peek()
	if err != nil {
		return err
	}
	if matchIdent(t, "UNBOUNDED") {
		_, _ = p.next()
		stmt.PasswordLockTime = "UNBOUNDED"
		return nil
	}
	n, err := p.readNumber()
	if err != nil {
		return err
	}
	stmt.PasswordLockTime = fmt.Sprintf("%d", n)
	return nil
}

// readNumber requires the next token to be a tkNumber and returns its
// integer value.
func (p *createUserParser) readNumber() (int, error) {
	t, err := p.next()
	if err != nil {
		return 0, err
	}
	if t.Kind != tkNumber {
		return 0, fmt.Errorf("parse: expected number, got %s", t)
	}
	var n int
	for _, c := range t.Value {
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// readString requires the next token to be a tkString and returns its
// decoded value.
func (p *createUserParser) readString() (string, error) {
	t, err := p.next()
	if err != nil {
		return "", err
	}
	if t.Kind != tkString {
		return "", fmt.Errorf("parse: expected quoted string, got %s", t)
	}
	return t.Value, nil
}

// equalsFoldAny is a small helper used in keyword matching. Wraps
// strings.EqualFold; kept local so callers read naturally.
func equalsFoldAny(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := 0; i < len(got); i++ {
		g := got[i]
		w := want[i]
		if 'A' <= g && g <= 'Z' {
			g += 'a' - 'A'
		}
		if 'A' <= w && w <= 'Z' {
			w += 'a' - 'A'
		}
		if g != w {
			return false
		}
	}
	return true
}
