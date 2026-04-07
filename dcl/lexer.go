// lexer.go implements a hand-written scanner that produces tokens from DCL source.
package dcl

import "strings"

// Lexer scans DCL source text into tokens.
type Lexer struct {
	filename string
	src      []byte
	pos      int // byte offset
	line     int // 1-based
	col      int // 1-based
	diags    Diagnostics
}

// NewLexer creates a Lexer for the given source.
func NewLexer(filename string, src []byte) *Lexer {
	return &Lexer{
		filename: filename,
		src:      src,
		pos:      0,
		line:     1,
		col:      1,
	}
}

// Diagnostics returns the diagnostics accumulated during scanning.
func (l *Lexer) Diagnostics() Diagnostics {
	return l.diags
}

// NextToken returns the next token and advances the lexer state.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.atEnd() {
		return Token{Type: TokenEOF, Pos: l.makePos()}
	}

	startPos := l.makePos()
	ch := l.peek()

	switch {
	case ch == '\n':
		l.advance()
		return Token{Type: TokenNewline, Literal: "\n", Pos: startPos}
	case ch == '#':
		return l.scanComment(startPos)
	case ch == '"':
		return l.scanString(startPos)
	case isDigit(ch):
		return l.scanNumber(startPos)
	case isIdentStart(ch):
		return l.scanIdentifier(startPos)
	case ch == '{':
		l.advance()
		return Token{Type: TokenLBrace, Literal: "{", Pos: startPos}
	case ch == '}':
		l.advance()
		return Token{Type: TokenRBrace, Literal: "}", Pos: startPos}
	case ch == '[':
		l.advance()
		return Token{Type: TokenLBracket, Literal: "[", Pos: startPos}
	case ch == ']':
		l.advance()
		return Token{Type: TokenRBracket, Literal: "]", Pos: startPos}
	case ch == '(':
		l.advance()
		return Token{Type: TokenLParen, Literal: "(", Pos: startPos}
	case ch == ')':
		l.advance()
		return Token{Type: TokenRParen, Literal: ")", Pos: startPos}
	case ch == '=':
		l.advance()
		return Token{Type: TokenEquals, Literal: "=", Pos: startPos}
	case ch == ',':
		l.advance()
		return Token{Type: TokenComma, Literal: ",", Pos: startPos}
	case ch == '.':
		l.advance()
		return Token{Type: TokenDot, Literal: ".", Pos: startPos}
	default:
		ch := l.advance()
		l.addDiagnostic(startPos, l.makePos(), "unexpected character: "+string(rune(ch)))
		return Token{Type: TokenIllegal, Literal: string(rune(ch)), Pos: startPos}
	}
}

// --- scan helpers ---

func (l *Lexer) scanComment(start Pos) Token {
	l.advance() // consume '#'
	var b strings.Builder
	for !l.atEnd() && l.peek() != '\n' {
		b.WriteByte(l.advance())
	}
	return Token{Type: TokenComment, Literal: b.String(), Pos: start}
}

func (l *Lexer) scanString(start Pos) Token {
	l.advance() // consume opening '"'
	var b strings.Builder
	for {
		if l.atEnd() {
			l.addDiagnostic(start, l.makePos(), "unterminated string")
			return Token{Type: TokenIllegal, Literal: b.String(), Pos: start}
		}
		ch := l.peek()
		if ch == '\n' {
			l.addDiagnostic(start, l.makePos(), "unterminated string")
			return Token{Type: TokenIllegal, Literal: b.String(), Pos: start}
		}
		if ch == '"' {
			l.advance() // consume closing '"'
			return Token{Type: TokenString, Literal: b.String(), Pos: start}
		}
		if ch == '$' && l.peekAt(1) == '{' {
			l.addDiagnosticWithSuggestion(start, l.makePos(),
				"string interpolation with ${} is not supported",
				"use secret() for sensitive values or a reference for resource attributes")
			// consume rest of string until closing quote, newline, or EOF
			for !l.atEnd() && l.peek() != '"' && l.peek() != '\n' {
				l.advance()
			}
			if !l.atEnd() && l.peek() == '"' {
				l.advance()
			}
			return Token{Type: TokenIllegal, Literal: b.String(), Pos: start}
		}
		if ch == '\\' {
			l.advance() // consume '\'
			if l.atEnd() {
				l.addDiagnostic(start, l.makePos(), "unterminated string")
				return Token{Type: TokenIllegal, Literal: b.String(), Pos: start}
			}
			esc := l.advance()
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			default:
				escPos := Pos{
					Filename: l.filename,
					Line:     l.line,
					Column:   l.col - 1, // point at the escaped char
					Offset:   l.pos - 1,
				}
				l.addDiagnostic(start, escPos, "unknown escape sequence: \\"+string(rune(esc)))
				return Token{Type: TokenIllegal, Literal: b.String(), Pos: start}
			}
			continue
		}
		b.WriteByte(l.advance())
	}
}

func (l *Lexer) scanNumber(start Pos) Token {
	var b strings.Builder
	for !l.atEnd() && isDigit(l.peek()) {
		b.WriteByte(l.advance())
	}
	// Check for float: '.' followed by digit
	if !l.atEnd() && l.peek() == '.' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1]) {
		b.WriteByte(l.advance()) // consume '.'
		for !l.atEnd() && isDigit(l.peek()) {
			b.WriteByte(l.advance())
		}
		return Token{Type: TokenFloat, Literal: b.String(), Pos: start}
	}
	return Token{Type: TokenInt, Literal: b.String(), Pos: start}
}

func (l *Lexer) scanIdentifier(start Pos) Token {
	var b strings.Builder
	for !l.atEnd() && isIdentPart(l.peek()) {
		b.WriteByte(l.advance())
	}
	lit := b.String()
	return Token{Type: lookupIdent(lit), Literal: lit, Pos: start}
}

// --- low-level helpers ---

func (l *Lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(offset int) byte {
	idx := l.pos + offset
	if idx >= len(l.src) {
		return 0
	}
	return l.src[idx]
}

func (l *Lexer) advance() byte {
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) atEnd() bool {
	return l.pos >= len(l.src)
}

func (l *Lexer) makePos() Pos {
	return Pos{
		Filename: l.filename,
		Line:     l.line,
		Column:   l.col,
		Offset:   l.pos,
	}
}

func (l *Lexer) skipWhitespace() {
	for !l.atEnd() {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) addDiagnostic(start, end Pos, message string) {
	l.diags = append(l.diags, Diagnostic{
		Severity: SeverityError,
		Message:  message,
		Range:    Range{Start: start, End: end},
	})
}

func (l *Lexer) addDiagnosticWithSuggestion(start, end Pos, message, suggestion string) {
	l.diags = append(l.diags, Diagnostic{
		Severity:   SeverityError,
		Message:    message,
		Range:      Range{Start: start, End: end},
		Suggestion: suggestion,
	})
}

// --- character classifiers ---

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
