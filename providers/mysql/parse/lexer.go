// Package parse implements parsers for MySQL DDL statements emitted by
// SHOW CREATE USER and SHOW GRANTS. The output of those statements
// across MySQL 8.0 and 8.4 is a stable contract, so the parsers here
// target exactly what the server emits — not the full MySQL grammar.
//
// The lexer handles the token shapes those statements use:
// backtick-quoted identifiers, single-quoted strings (with SQL escape
// sequences), bare identifiers/keywords, integers, and a handful of
// single-character punctuators.
package parse

import (
	"fmt"
	"strings"
	"unicode"
)

// tokenKind enumerates the tokens the lexer can produce.
type tokenKind int

const (
	tkEOF tokenKind = iota
	tkIdent
	tkString   // single-quoted, with SQL escapes decoded
	tkBacktick // backtick-quoted identifier, escapes decoded
	tkNumber   // unsigned decimal integer
	tkAt       // @
	tkDot      // .
	tkComma    // ,
	tkLParen   // (
	tkRParen   // )
	tkSemi     // ;
	tkStar     // *
)

// token is a lexed unit: kind plus the textual value (for idents,
// strings, and numbers). Punctuation tokens carry an empty Value.
type token struct {
	Kind  tokenKind
	Value string
	Pos   int // offset into the source
}

func (t token) String() string {
	if t.Value != "" {
		return fmt.Sprintf("%s(%q)", kindName(t.Kind), t.Value)
	}
	return kindName(t.Kind)
}

func kindName(k tokenKind) string {
	switch k {
	case tkEOF:
		return "EOF"
	case tkIdent:
		return "IDENT"
	case tkString:
		return "STRING"
	case tkBacktick:
		return "BACKTICK"
	case tkNumber:
		return "NUMBER"
	case tkAt:
		return "@"
	case tkDot:
		return "."
	case tkComma:
		return ","
	case tkLParen:
		return "("
	case tkRParen:
		return ")"
	case tkSemi:
		return ";"
	case tkStar:
		return "*"
	}
	return "?"
}

// lexer walks a source string producing tokens.
type lexer struct {
	src string
	pos int
}

// newLexer constructs a lexer positioned at the start of src.
func newLexer(src string) *lexer {
	return &lexer{src: src}
}

// next returns the next token or an error.
func (l *lexer) next() (token, error) {
	l.skipWhitespace()
	if l.pos >= len(l.src) {
		return token{Kind: tkEOF, Pos: l.pos}, nil
	}
	start := l.pos
	c := l.src[l.pos]
	switch {
	case c == '`':
		return l.readBacktick()
	case c == '\'':
		return l.readString()
	case c == '@':
		l.pos++
		return token{Kind: tkAt, Pos: start}, nil
	case c == '.':
		l.pos++
		return token{Kind: tkDot, Pos: start}, nil
	case c == ',':
		l.pos++
		return token{Kind: tkComma, Pos: start}, nil
	case c == '(':
		l.pos++
		return token{Kind: tkLParen, Pos: start}, nil
	case c == ')':
		l.pos++
		return token{Kind: tkRParen, Pos: start}, nil
	case c == ';':
		l.pos++
		return token{Kind: tkSemi, Pos: start}, nil
	case c == '*':
		l.pos++
		return token{Kind: tkStar, Pos: start}, nil
	case c >= '0' && c <= '9':
		return l.readNumber()
	case isIdentStart(c):
		return l.readIdent()
	}
	return token{}, fmt.Errorf("parse: unexpected character %q at offset %d", c, start)
}

// peek returns the next token without consuming it.
func (l *lexer) peek() (token, error) {
	save := l.pos
	t, err := l.next()
	l.pos = save
	return t, err
}

func (l *lexer) skipWhitespace() {
	for l.pos < len(l.src) && unicode.IsSpace(rune(l.src[l.pos])) {
		l.pos++
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func (l *lexer) readIdent() (token, error) {
	start := l.pos
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.pos++
	}
	return token{Kind: tkIdent, Value: l.src[start:l.pos], Pos: start}, nil
}

func (l *lexer) readNumber() (token, error) {
	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
		l.pos++
	}
	return token{Kind: tkNumber, Value: l.src[start:l.pos], Pos: start}, nil
}

// readBacktick consumes a `backtick`-quoted identifier. Doubled
// backticks escape a literal backtick inside the identifier.
func (l *lexer) readBacktick() (token, error) {
	start := l.pos
	l.pos++ // opening backtick
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '`' {
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '`' {
				b.WriteByte('`')
				l.pos += 2
				continue
			}
			l.pos++ // closing backtick
			return token{Kind: tkBacktick, Value: b.String(), Pos: start}, nil
		}
		b.WriteByte(c)
		l.pos++
	}
	return token{}, fmt.Errorf("parse: unterminated backtick identifier at offset %d", start)
}

// readString consumes a 'single'-quoted string and decodes SQL escape
// sequences. Doubled single quotes ('') produce a literal quote.
func (l *lexer) readString() (token, error) {
	start := l.pos
	l.pos++ // opening quote
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\'' {
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '\'' {
				b.WriteByte('\'')
				l.pos += 2
				continue
			}
			l.pos++ // closing quote
			return token{Kind: tkString, Value: b.String(), Pos: start}, nil
		}
		if c == '\\' && l.pos+1 < len(l.src) {
			esc := l.src[l.pos+1]
			switch esc {
			case '0':
				b.WriteByte(0)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'Z':
				b.WriteByte(0x1a)
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			case '"':
				b.WriteByte('"')
			default:
				// Unknown escape: MySQL keeps the character as-is.
				b.WriteByte(esc)
			}
			l.pos += 2
			continue
		}
		b.WriteByte(c)
		l.pos++
	}
	return token{}, fmt.Errorf("parse: unterminated string at offset %d", start)
}

// matchIdent reports whether t is a bare IDENT whose value equals name
// case-insensitively. DDL keywords are case-insensitive in MySQL.
func matchIdent(t token, name string) bool {
	return t.Kind == tkIdent && strings.EqualFold(t.Value, name)
}
