package dcl

import "testing"

// collectTokens drains the lexer until EOF and returns all tokens (including EOF).
func collectTokens(l *Lexer) []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			return tokens
		}
	}
}

func TestLexerEmptyInput(t *testing.T) {
	l := NewLexer("test.dcl", []byte(""))
	tok := l.NextToken()
	if tok.Type != TokenEOF {
		t.Fatalf("expected EOF, got %v", tok)
	}
	if tok.Pos.Line != 1 || tok.Pos.Column != 1 {
		t.Errorf("EOF pos = %d:%d, want 1:1", tok.Pos.Line, tok.Pos.Column)
	}
}

func TestLexerComment(t *testing.T) {
	l := NewLexer("", []byte("# this is a comment"))
	tok := l.NextToken()
	if tok.Type != TokenComment {
		t.Fatalf("expected Comment, got %v", tok)
	}
	if tok.Literal != " this is a comment" {
		t.Errorf("literal = %q, want %q", tok.Literal, " this is a comment")
	}
}

func TestLexerStringLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"hello"`, "hello"},
		{`""`, ""},
	}
	for _, tc := range tests {
		l := NewLexer("", []byte(tc.input))
		tok := l.NextToken()
		if tok.Type != TokenString {
			t.Errorf("input %s: expected String, got %v", tc.input, tok)
			continue
		}
		if tok.Literal != tc.want {
			t.Errorf("input %s: literal = %q, want %q", tc.input, tok.Literal, tc.want)
		}
	}
}

func TestLexerStringEscapes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"hello\nworld"`, "hello\nworld"},
		{`"tab\there"`, "tab\there"},
		{`"back\\slash"`, "back\\slash"},
		{`"say \"hi\""`, `say "hi"`},
	}
	for _, tc := range tests {
		l := NewLexer("", []byte(tc.input))
		tok := l.NextToken()
		if tok.Type != TokenString {
			t.Errorf("input %s: expected String, got %v", tc.input, tok)
			continue
		}
		if tok.Literal != tc.want {
			t.Errorf("input %s: literal = %q, want %q", tc.input, tok.Literal, tc.want)
		}
	}
}

func TestLexerStringInterpolation(t *testing.T) {
	l := NewLexer("", []byte(`"hello ${name}"`))
	tok := l.NextToken()
	if tok.Type != TokenIllegal {
		t.Fatalf("expected Illegal, got %v", tok)
	}
	diags := l.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for string interpolation")
	}
	if diags[0].Message != "string interpolation with ${} is not supported" {
		t.Errorf("message = %q", diags[0].Message)
	}
	if diags[0].Suggestion == "" {
		t.Error("expected suggestion on interpolation diagnostic")
	}
}

func TestLexerUnterminatedString(t *testing.T) {
	tests := []string{`"hello`, `"hello\`}
	for _, input := range tests {
		l := NewLexer("", []byte(input))
		tok := l.NextToken()
		if tok.Type != TokenIllegal {
			t.Errorf("input %q: expected Illegal, got %v", input, tok)
		}
		if len(l.Diagnostics()) == 0 {
			t.Errorf("input %q: expected diagnostic", input)
		}
	}
}

func TestLexerIntegers(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"42", "42"},
		{"0", "0"},
	}
	for _, tc := range tests {
		l := NewLexer("", []byte(tc.input))
		tok := l.NextToken()
		if tok.Type != TokenInt {
			t.Errorf("input %s: expected Int, got %v", tc.input, tok)
		}
		if tok.Literal != tc.want {
			t.Errorf("input %s: literal = %q, want %q", tc.input, tok.Literal, tc.want)
		}
	}
}

func TestLexerFloats(t *testing.T) {
	l := NewLexer("", []byte("3.14"))
	tok := l.NextToken()
	if tok.Type != TokenFloat {
		t.Fatalf("expected Float, got %v", tok)
	}
	if tok.Literal != "3.14" {
		t.Errorf("literal = %q, want %q", tok.Literal, "3.14")
	}
}

func TestLexerNumberEdgeCases(t *testing.T) {
	// 123.abc → Int("123"), Dot, Ident("abc")
	l := NewLexer("", []byte("123.abc"))
	tokens := collectTokens(l)
	expected := []struct {
		tt  TokenType
		lit string
	}{
		{TokenInt, "123"},
		{TokenDot, "."},
		{TokenIdent, "abc"},
		{TokenEOF, ""},
	}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, e := range expected {
		if tokens[i].Type != e.tt || tokens[i].Literal != e.lit {
			t.Errorf("token[%d] = %v(%q), want %v(%q)", i, tokens[i].Type, tokens[i].Literal, e.tt, e.lit)
		}
	}

	// 123. → Int("123"), Dot
	l2 := NewLexer("", []byte("123."))
	tokens2 := collectTokens(l2)
	if tokens2[0].Type != TokenInt || tokens2[0].Literal != "123" {
		t.Errorf("token[0] = %v(%q), want Int(123)", tokens2[0].Type, tokens2[0].Literal)
	}
	if tokens2[1].Type != TokenDot {
		t.Errorf("token[1] = %v, want Dot", tokens2[1].Type)
	}
}

func TestLexerBooleans(t *testing.T) {
	l := NewLexer("", []byte("true false"))
	tok1 := l.NextToken()
	tok2 := l.NextToken()
	if tok1.Type != TokenTrue || tok1.Literal != "true" {
		t.Errorf("expected True(true), got %v(%q)", tok1.Type, tok1.Literal)
	}
	if tok2.Type != TokenFalse || tok2.Literal != "false" {
		t.Errorf("expected False(false), got %v(%q)", tok2.Type, tok2.Literal)
	}
}

func TestLexerIdentifiers(t *testing.T) {
	tests := []string{"foo", "_bar", "x123"}
	for _, input := range tests {
		l := NewLexer("", []byte(input))
		tok := l.NextToken()
		if tok.Type != TokenIdent {
			t.Errorf("input %q: expected Ident, got %v", input, tok)
		}
		if tok.Literal != input {
			t.Errorf("input %q: literal = %q", input, tok.Literal)
		}
	}
}

func TestLexerPunctuation(t *testing.T) {
	tests := []struct {
		input string
		want  TokenType
	}{
		{"{", TokenLBrace},
		{"}", TokenRBrace},
		{"[", TokenLBracket},
		{"]", TokenRBracket},
		{"(", TokenLParen},
		{")", TokenRParen},
		{"=", TokenEquals},
		{",", TokenComma},
		{".", TokenDot},
	}
	for _, tc := range tests {
		l := NewLexer("", []byte(tc.input))
		tok := l.NextToken()
		if tok.Type != tc.want {
			t.Errorf("input %q: expected %v, got %v", tc.input, tc.want, tok.Type)
		}
	}
}

func TestLexerMultiTokenSequence(t *testing.T) {
	input := `context "prod" {
  region = "us-east-1"
}`
	l := NewLexer("test.dcl", []byte(input))
	tokens := collectTokens(l)

	expected := []TokenType{
		TokenIdent,   // context
		TokenString,  // "prod"
		TokenLBrace,  // {
		TokenNewline, // \n
		TokenIdent,   // region
		TokenEquals,  // =
		TokenString,  // "us-east-1"
		TokenNewline, // \n
		TokenRBrace,  // }
		TokenEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d:\n%v", len(tokens), len(expected), tokens)
	}
	for i, e := range expected {
		if tokens[i].Type != e {
			t.Errorf("token[%d] = %v, want %v", i, tokens[i].Type, e)
		}
	}
}

func TestLexerPositionTracking(t *testing.T) {
	input := "abc\ndef\nghi"
	l := NewLexer("", []byte(input))
	tokens := collectTokens(l)

	// abc at 1:1, \n at 1:4, def at 2:1, \n at 2:4, ghi at 3:1, EOF at 3:4
	wantPositions := []struct {
		line, col int
	}{
		{1, 1}, // abc
		{1, 4}, // \n
		{2, 1}, // def
		{2, 4}, // \n
		{3, 1}, // ghi
		{3, 4}, // EOF
	}
	if len(tokens) != len(wantPositions) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(wantPositions))
	}
	for i, wp := range wantPositions {
		if tokens[i].Pos.Line != wp.line || tokens[i].Pos.Column != wp.col {
			t.Errorf("token[%d] (%v) pos = %d:%d, want %d:%d",
				i, tokens[i].Type, tokens[i].Pos.Line, tokens[i].Pos.Column, wp.line, wp.col)
		}
	}
}

func TestLexerIllegalCharacter(t *testing.T) {
	l := NewLexer("", []byte("@"))
	tok := l.NextToken()
	if tok.Type != TokenIllegal {
		t.Fatalf("expected Illegal, got %v", tok)
	}
	if tok.Literal != "@" {
		t.Errorf("literal = %q, want %q", tok.Literal, "@")
	}
	if len(l.Diagnostics()) == 0 {
		t.Error("expected diagnostic for illegal character")
	}
}

func TestLexerWhitespaceHandling(t *testing.T) {
	l := NewLexer("", []byte("  \t  foo  \t  "))
	tokens := collectTokens(l)
	// Should produce: Ident("foo"), EOF
	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2: %v", len(tokens), tokens)
	}
	if tokens[0].Type != TokenIdent || tokens[0].Literal != "foo" {
		t.Errorf("token[0] = %v(%q), want Ident(foo)", tokens[0].Type, tokens[0].Literal)
	}
	if tokens[1].Type != TokenEOF {
		t.Errorf("token[1] = %v, want EOF", tokens[1].Type)
	}
}

func TestLexerNewlineVariants(t *testing.T) {
	// \r\n should produce single newline (\r is whitespace, \n is newline token)
	l := NewLexer("", []byte("\r\n"))
	tokens := collectTokens(l)
	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2: %v", len(tokens), tokens)
	}
	if tokens[0].Type != TokenNewline {
		t.Errorf("token[0] = %v, want Newline", tokens[0].Type)
	}

	// \n\n produces two newlines
	l2 := NewLexer("", []byte("\n\n"))
	tokens2 := collectTokens(l2)
	if len(tokens2) != 3 { // Newline, Newline, EOF
		t.Fatalf("got %d tokens, want 3: %v", len(tokens2), tokens2)
	}
	if tokens2[0].Type != TokenNewline || tokens2[1].Type != TokenNewline {
		t.Errorf("expected two Newline tokens, got %v %v", tokens2[0].Type, tokens2[1].Type)
	}
}

func TestLexerStringWithNewline(t *testing.T) {
	// A literal newline inside a string is unterminated
	l := NewLexer("", []byte("\"hello\nworld\""))
	tok := l.NextToken()
	if tok.Type != TokenIllegal {
		t.Fatalf("expected Illegal, got %v", tok)
	}
	diags := l.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for unterminated string")
	}
	if diags[0].Message != "unterminated string" {
		t.Errorf("message = %q, want %q", diags[0].Message, "unterminated string")
	}
}

func TestLexerRepeatedEOF(t *testing.T) {
	l := NewLexer("", []byte(""))
	for i := 0; i < 3; i++ {
		tok := l.NextToken()
		if tok.Type != TokenEOF {
			t.Errorf("call %d: expected EOF, got %v", i+1, tok)
		}
	}
}

func TestLexerUnknownEscape(t *testing.T) {
	l := NewLexer("", []byte(`"\a"`))
	tok := l.NextToken()
	if tok.Type != TokenIllegal {
		t.Fatalf("expected Illegal, got %v", tok)
	}
	diags := l.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for unknown escape")
	}
	if diags[0].Message != `unknown escape sequence: \a` {
		t.Errorf("message = %q", diags[0].Message)
	}
}
