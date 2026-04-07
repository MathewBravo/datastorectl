// token.go defines the TokenType enum and Token struct used by the lexer.
package dcl

import "fmt"

// TokenType classifies a lexical token.
type TokenType int

const (
	TokenIllegal  TokenType = iota // zero value — uninitialised or invalid
	TokenEOF                       // end of input
	TokenNewline                   // \n (significant for statement termination)
	TokenComment                   // # …
	TokenIdent                     // identifier
	TokenString                    // "…"
	TokenInt                       // 123
	TokenFloat                     // 3.14
	TokenTrue                      // true
	TokenFalse                     // false
	TokenLBrace                    // {
	TokenRBrace                    // }
	TokenLBracket                  // [
	TokenRBracket                  // ]
	TokenLParen                    // (
	TokenRParen                    // )
	TokenEquals                    // =
	TokenComma                     // ,
	TokenDot                       // .
)

func (t TokenType) String() string {
	switch t {
	case TokenIllegal:
		return "Illegal"
	case TokenEOF:
		return "EOF"
	case TokenNewline:
		return "Newline"
	case TokenComment:
		return "Comment"
	case TokenIdent:
		return "Ident"
	case TokenString:
		return "String"
	case TokenInt:
		return "Int"
	case TokenFloat:
		return "Float"
	case TokenTrue:
		return "True"
	case TokenFalse:
		return "False"
	case TokenLBrace:
		return "LBrace"
	case TokenRBrace:
		return "RBrace"
	case TokenLBracket:
		return "LBracket"
	case TokenRBracket:
		return "RBracket"
	case TokenLParen:
		return "LParen"
	case TokenRParen:
		return "RParen"
	case TokenEquals:
		return "Equals"
	case TokenComma:
		return "Comma"
	case TokenDot:
		return "Dot"
	default:
		return fmt.Sprintf("TokenType(%d)", int(t))
	}
}

// Token represents a single lexical token with its source position.
type Token struct {
	Type    TokenType
	Literal string
	Pos     Pos
}

func (t Token) String() string {
	if t.Literal == "" {
		return t.Type.String()
	}
	return fmt.Sprintf("%s(%q)", t.Type, t.Literal)
}

// keywords maps reserved words to their token types.
var keywords = map[string]TokenType{
	"true":  TokenTrue,
	"false": TokenFalse,
}

// lookupIdent returns the keyword TokenType for ident, or TokenIdent.
func lookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TokenIdent
}
