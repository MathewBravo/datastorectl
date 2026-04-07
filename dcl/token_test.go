package dcl

import "testing"

func TestTokenTypeString(t *testing.T) {
	tests := []struct {
		tt   TokenType
		want string
	}{
		{TokenIllegal, "Illegal"},
		{TokenEOF, "EOF"},
		{TokenNewline, "Newline"},
		{TokenComment, "Comment"},
		{TokenIdent, "Ident"},
		{TokenString, "String"},
		{TokenInt, "Int"},
		{TokenFloat, "Float"},
		{TokenTrue, "True"},
		{TokenFalse, "False"},
		{TokenLBrace, "LBrace"},
		{TokenRBrace, "RBrace"},
		{TokenLBracket, "LBracket"},
		{TokenRBracket, "RBracket"},
		{TokenLParen, "LParen"},
		{TokenRParen, "RParen"},
		{TokenEquals, "Equals"},
		{TokenComma, "Comma"},
		{TokenDot, "Dot"},
		{TokenType(999), "TokenType(999)"},
	}
	for _, tc := range tests {
		if got := tc.tt.String(); got != tc.want {
			t.Errorf("TokenType(%d).String() = %q, want %q", int(tc.tt), got, tc.want)
		}
	}
}

func TestTokenString(t *testing.T) {
	tok := Token{Type: TokenIdent, Literal: "foo"}
	if got := tok.String(); got != `Ident("foo")` {
		t.Errorf("Token.String() = %q, want %q", got, `Ident("foo")`)
	}

	eof := Token{Type: TokenEOF}
	if got := eof.String(); got != "EOF" {
		t.Errorf("Token.String() = %q, want %q", got, "EOF")
	}
}

func TestLookupIdent(t *testing.T) {
	tests := []struct {
		ident string
		want  TokenType
	}{
		{"true", TokenTrue},
		{"false", TokenFalse},
		{"context", TokenIdent},
		{"TRUE", TokenIdent}, // case-sensitive
		{"False", TokenIdent},
	}
	for _, tc := range tests {
		if got := lookupIdent(tc.ident); got != tc.want {
			t.Errorf("lookupIdent(%q) = %v, want %v", tc.ident, got, tc.want)
		}
	}
}
