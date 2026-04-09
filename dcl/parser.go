// parser.go implements a recursive-descent parser that converts tokens into an AST.
package dcl

import (
	"fmt"
	"strconv"
)

const maxErrors = 20

// Parse parses DCL source into an AST File node.
func Parse(filename string, src []byte) (*File, Diagnostics) {
	lex := NewLexer(filename, src)
	p := &parser{lex: lex}

	// Prime two-token lookahead buffer.
	p.cur = p.readNonComment()
	p.peek = p.readNonComment()

	file := p.parseFile()

	// Merge lexer diagnostics into parser diagnostics.
	all := make(Diagnostics, 0, len(lex.Diagnostics())+len(p.diags))
	all = append(all, lex.Diagnostics()...)
	all = append(all, p.diags...)
	file.Diagnostics = all

	return file, all
}

type parser struct {
	lex           *Lexer
	cur           Token // current token
	peek          Token // one-token lookahead
	diags         Diagnostics
	errCount      int
	tooManyErrors bool
}

// tokenEnd computes the end Pos from a token's start position and literal length.
// For strings it adds 2 to account for the surrounding quotes (which are stripped
// from Token.Literal by the lexer). Single-line tokens only.
func tokenEnd(tok Token) Pos {
	length := len(tok.Literal)
	if tok.Type == TokenString {
		length += 2 // account for quotes
	}
	return Pos{
		Filename: tok.Pos.Filename,
		Line:     tok.Pos.Line,
		Column:   tok.Pos.Column + length,
		Offset:   tok.Pos.Offset + length,
	}
}

// --- token management ---

// readNonComment reads the next token from the lexer, skipping comments.
func (p *parser) readNonComment() Token {
	for {
		tok := p.lex.NextToken()
		if tok.Type != TokenComment {
			return tok
		}
	}
}

// nextToken shifts peek→cur and reads a new peek.
func (p *parser) nextToken() {
	p.cur = p.peek
	p.peek = p.readNonComment()
}

// skipNewlines consumes consecutive newline tokens.
func (p *parser) skipNewlines() {
	for p.cur.Type == TokenNewline {
		p.nextToken()
	}
}

// expect asserts the current token is of the given type, advances, and returns it.
// If the type doesn't match, it records an error and returns ok=false.
func (p *parser) expect(typ TokenType) (Token, bool) {
	if p.cur.Type == typ {
		tok := p.cur
		p.nextToken()
		return tok, true
	}
	p.addError(p.cur.Pos, fmt.Sprintf("expected %s, got %s", typ, p.cur.Type))
	return p.cur, false
}

// addError appends an error diagnostic at the given position.
func (p *parser) addError(pos Pos, msg string) {
	if p.tooManyErrors {
		return
	}
	p.diags = append(p.diags, Diagnostic{
		Severity: SeverityError,
		Message:  msg,
		Range:    Range{Start: pos, End: pos},
	})
	p.errCount++
	if p.errCount >= maxErrors {
		p.tooManyErrors = true
		p.diags = append(p.diags, Diagnostic{
			Severity: SeverityError,
			Message:  "too many errors",
			Range:    Range{Start: pos, End: pos},
		})
	}
}

// --- error recovery ---

// recoverToNextStatement skips tokens until a newline, }, or EOF.
// If a newline is found it is consumed.
func (p *parser) recoverToNextStatement() {
	for p.cur.Type != TokenNewline && p.cur.Type != TokenRBrace && p.cur.Type != TokenEOF {
		p.nextToken()
	}
	if p.cur.Type == TokenNewline {
		p.nextToken()
	}
}

// recoverToBlockEnd skips tokens until the matching closing brace (tracking depth).
func (p *parser) recoverToBlockEnd() {
	depth := 1
	for depth > 0 && p.cur.Type != TokenEOF {
		if p.cur.Type == TokenLBrace {
			depth++
		} else if p.cur.Type == TokenRBrace {
			depth--
		}
		if depth > 0 {
			p.nextToken()
		}
	}
	// Consume the closing brace if present.
	if p.cur.Type == TokenRBrace {
		p.nextToken()
	}
}

// recoverToListEnd skips tokens until the matching closing bracket (tracking depth).
func (p *parser) recoverToListEnd() {
	depth := 1
	for depth > 0 && p.cur.Type != TokenEOF {
		if p.cur.Type == TokenLBracket {
			depth++
		} else if p.cur.Type == TokenRBracket {
			depth--
		}
		if depth > 0 {
			p.nextToken()
		}
	}
	// Consume the closing bracket if present.
	if p.cur.Type == TokenRBracket {
		p.nextToken()
	}
}

// --- parsing methods ---

// parseFile parses the top-level structure: a sequence of blocks.
func (p *parser) parseFile() *File {
	fileStart := p.cur.Pos
	var blocks []Block

	for !p.tooManyErrors {
		p.skipNewlines()

		if p.cur.Type == TokenEOF {
			break
		}

		if p.cur.Type != TokenIdent {
			p.addError(p.cur.Pos, fmt.Sprintf("expected block type (identifier), got %s", p.cur.Type))
			p.recoverToNextStatement()
			continue
		}

		// Disambiguation: ident + peek = → top-level attribute error.
		if p.peek.Type == TokenEquals {
			p.addError(p.cur.Pos, "attributes are not allowed at the top level")
			p.recoverToNextStatement()
			continue
		}

		// ident + peek string/{ → block.
		block, ok := p.parseBlock()
		if ok {
			blocks = append(blocks, block)
		}
	}

	fileEnd := p.cur.Pos // EOF position
	return &File{
		Blocks: blocks,
		Rng:    Range{Start: fileStart, End: fileEnd},
	}
}

// parseBlock parses: type ["label"] { attrs... }
func (p *parser) parseBlock() (Block, bool) {
	typeToken := p.cur
	p.nextToken() // consume type identifier

	var label string
	if p.cur.Type == TokenString {
		label = p.cur.Literal
		p.nextToken()
	}

	// Expect opening brace.
	if _, ok := p.expect(TokenLBrace); !ok {
		p.recoverToNextStatement()
		return Block{
			Type:  typeToken.Literal,
			Label: label,
			Rng:   Range{Start: typeToken.Pos, End: p.cur.Pos},
		}, true
	}

	var attrs []Attribute
	var blocks []Block

	for !p.tooManyErrors {
		p.skipNewlines()

		if p.cur.Type == TokenRBrace || p.cur.Type == TokenEOF {
			break
		}

		if p.cur.Type != TokenIdent {
			p.addError(p.cur.Pos, fmt.Sprintf("expected attribute name (identifier), got %s", p.cur.Type))
			p.recoverToNextStatement()
			continue
		}

		// Disambiguation inside block body.
		if p.peek.Type == TokenEquals {
			attr, ok := p.parseAttribute()
			if ok {
				attrs = append(attrs, attr)
			}
			continue
		}

		// ident + peek string/{ → nested block (recursive).
		if p.peek.Type == TokenString || p.peek.Type == TokenLBrace {
			block, ok := p.parseBlock()
			if ok {
				blocks = append(blocks, block)
			}
			continue
		}

		p.addError(p.cur.Pos, fmt.Sprintf("expected '=' after attribute name, got %s", p.peek.Type))
		p.recoverToNextStatement()
	}

	if p.cur.Type == TokenRBrace {
		rbrace := p.cur
		p.nextToken()
		return Block{
			Type:       typeToken.Literal,
			Label:      label,
			Attributes: attrs,
			Blocks:     blocks,
			Rng:        Range{Start: typeToken.Pos, End: tokenEnd(rbrace)},
		}, true
	}

	// Missing closing brace — return partial block with what we parsed.
	p.addError(typeToken.Pos,
		fmt.Sprintf("missing closing '}' for block %q", typeToken.Literal))
	p.recoverToBlockEnd()
	return Block{
		Type:       typeToken.Literal,
		Label:      label,
		Attributes: attrs,
		Blocks:     blocks,
		Rng:        Range{Start: typeToken.Pos, End: p.cur.Pos},
	}, true
}

// parseAttribute parses: key = value
func (p *parser) parseAttribute() (Attribute, bool) {
	keyToken := p.cur
	p.nextToken() // consume key

	p.nextToken() // consume '='

	expr, ok := p.parseExpression()
	if !ok {
		p.recoverToNextStatement()
		return Attribute{}, false
	}

	return Attribute{
		Key:   keyToken.Literal,
		Value: expr,
		Rng:   Range{Start: keyToken.Pos, End: expr.nodeRange().End},
	}, true
}

// parseExpression parses a literal value: string, int, float, or bool.
func (p *parser) parseExpression() (Expression, bool) {
	tok := p.cur
	switch tok.Type {
	case TokenString:
		p.nextToken()
		return &LiteralString{
			Value: tok.Literal,
			Rng:   Range{Start: tok.Pos, End: tokenEnd(tok)},
		}, true

	case TokenInt:
		val, err := strconv.ParseInt(tok.Literal, 10, 64)
		if err != nil {
			p.addError(tok.Pos, fmt.Sprintf("invalid integer: %s", tok.Literal))
			p.nextToken()
			return nil, false
		}
		p.nextToken()
		return &LiteralInt{
			Value: val,
			Rng:   Range{Start: tok.Pos, End: tokenEnd(tok)},
		}, true

	case TokenFloat:
		val, err := strconv.ParseFloat(tok.Literal, 64)
		if err != nil {
			p.addError(tok.Pos, fmt.Sprintf("invalid float: %s", tok.Literal))
			p.nextToken()
			return nil, false
		}
		p.nextToken()
		return &LiteralFloat{
			Value: val,
			Rng:   Range{Start: tok.Pos, End: tokenEnd(tok)},
		}, true

	case TokenTrue:
		p.nextToken()
		return &LiteralBool{
			Value: true,
			Rng:   Range{Start: tok.Pos, End: tokenEnd(tok)},
		}, true

	case TokenFalse:
		p.nextToken()
		return &LiteralBool{
			Value: false,
			Rng:   Range{Start: tok.Pos, End: tokenEnd(tok)},
		}, true

	case TokenIdent:
		parts := []string{tok.Literal}
		end := tokenEnd(tok)
		for p.peek.Type == TokenDot {
			p.nextToken() // advance to dot
			p.nextToken() // advance past dot
			if p.cur.Type != TokenIdent {
				p.addError(p.cur.Pos, fmt.Sprintf("expected identifier after '.', got %s", p.cur.Type))
				return nil, false
			}
			parts = append(parts, p.cur.Literal)
			end = tokenEnd(p.cur)
		}
		p.nextToken() // advance past last identifier
		// Function call: only for single-part identifiers followed by '('
		if len(parts) == 1 && p.cur.Type == TokenLParen {
			return p.parseFunctionCall(parts[0], tok.Pos)
		}
		if len(parts) == 1 {
			return &Identifier{Name: parts[0], Rng: Range{Start: tok.Pos, End: end}}, true
		}
		return &Reference{Parts: parts, Rng: Range{Start: tok.Pos, End: end}}, true

	case TokenLBracket:
		return p.parseList()

	case TokenLBrace:
		return p.parseMap()

	default:
		p.addError(tok.Pos, fmt.Sprintf("expected value, got %s", tok.Type))
		return nil, false
	}
}

// parseList parses: [ elem, elem, ... ]
func (p *parser) parseList() (Expression, bool) {
	lbracket := p.cur
	p.nextToken() // consume '['
	p.skipNewlines()

	var elems []Expression

	for p.cur.Type != TokenRBracket && p.cur.Type != TokenRBrace && p.cur.Type != TokenEOF {
		expr, ok := p.parseExpression()
		if !ok {
			p.recoverToListEnd()
			return nil, false
		}
		elems = append(elems, expr)

		p.skipNewlines()
		if p.cur.Type == TokenComma {
			p.nextToken() // consume ','
			p.skipNewlines()
			if p.cur.Type == TokenRBracket {
				break // trailing comma
			}
		}
	}

	rbracket, ok := p.expect(TokenRBracket)
	if !ok {
		p.recoverToListEnd()
		return nil, false
	}

	return &ListExpr{
		Elements: elems,
		Rng:      Range{Start: lbracket.Pos, End: tokenEnd(rbracket)},
	}, true
}

// parseMap parses: { key = value, key = value, ... }
func (p *parser) parseMap() (Expression, bool) {
	lbrace := p.cur
	p.nextToken() // consume '{'
	p.skipNewlines()

	var keys []string
	var values []Expression

	for p.cur.Type != TokenRBrace && p.cur.Type != TokenEOF {
		if !p.parseMapEntry(&keys, &values) {
			p.recoverToBlockEnd()
			return nil, false
		}

		p.skipNewlines()
		if p.cur.Type == TokenComma {
			p.nextToken() // consume ','
			p.skipNewlines()
			if p.cur.Type == TokenRBrace {
				break // trailing comma
			}
		}
	}

	rbrace, ok := p.expect(TokenRBrace)
	if !ok {
		p.recoverToBlockEnd()
		return nil, false
	}

	return &MapExpr{
		Keys:   keys,
		Values: values,
		Rng:    Range{Start: lbrace.Pos, End: tokenEnd(rbrace)},
	}, true
}

// parseFunctionCall parses: name( arg, arg, ... )
// The opening '(' is p.cur when called.
func (p *parser) parseFunctionCall(name string, start Pos) (Expression, bool) {
	p.nextToken() // consume '('
	p.skipNewlines()

	var args []Expression

	// Empty arg list
	if p.cur.Type == TokenRParen {
		end := tokenEnd(p.cur)
		p.nextToken() // consume ')'
		return &FunctionCall{Name: name, Args: args, Rng: Range{Start: start, End: end}}, true
	}

	// Parse first argument
	arg, ok := p.parseExpression()
	if !ok {
		return nil, false
	}
	args = append(args, arg)

	// Parse remaining comma-separated arguments
	for p.cur.Type == TokenComma {
		p.nextToken() // consume ','
		p.skipNewlines()
		// Trailing comma: next token is ')'
		if p.cur.Type == TokenRParen {
			break
		}
		arg, ok = p.parseExpression()
		if !ok {
			return nil, false
		}
		args = append(args, arg)
	}

	p.skipNewlines()
	if p.cur.Type != TokenRParen {
		p.addError(p.cur.Pos, fmt.Sprintf("expected ',' or ')' in argument list, got %s", p.cur.Type))
		return nil, false
	}
	end := tokenEnd(p.cur)
	p.nextToken() // consume ')'
	return &FunctionCall{Name: name, Args: args, Rng: Range{Start: start, End: end}}, true
}

// parseMapEntry parses a single: ident = expr
func (p *parser) parseMapEntry(keys *[]string, values *[]Expression) bool {
	if p.cur.Type != TokenIdent {
		p.addError(p.cur.Pos, fmt.Sprintf("expected map key (identifier), got %s", p.cur.Type))
		return false
	}
	key := p.cur.Literal
	p.nextToken() // consume key

	if _, ok := p.expect(TokenEquals); !ok {
		return false
	}

	val, ok := p.parseExpression()
	if !ok {
		return false
	}

	*keys = append(*keys, key)
	*values = append(*values, val)
	return true
}
