// ast.go defines the AST node types produced by the parser.
package dcl

// Node — all AST nodes carry a source Range. Unexported to close the set.
type Node interface {
	nodeRange() Range
}

// Expression — marker for nodes that can appear as values. Embeds Node.
type Expression interface {
	Node
	exprNode()
}

// --- Structural types ---

// File is the top-level AST node representing an entire DCL source file.
type File struct {
	Blocks      []Block
	Diagnostics Diagnostics
	Rng         Range
}

func (f *File) nodeRange() Range { return f.Rng }

// Block represents a labeled or unlabeled block (e.g. "resource db { ... }").
type Block struct {
	Type       string
	Label      string // empty = unlabeled
	Attributes []Attribute
	Blocks     []Block
	Rng        Range
}

func (b *Block) nodeRange() Range { return b.Rng }

// Attribute represents a key = value pair inside a block.
type Attribute struct {
	Key   string
	Value Expression
	Rng   Range
}

func (a *Attribute) nodeRange() Range { return a.Rng }

// --- Expression types ---

// LiteralString represents a string literal.
type LiteralString struct {
	Value string
	Rng   Range
}

func (l *LiteralString) nodeRange() Range { return l.Rng }
func (l *LiteralString) exprNode()        {}

// LiteralInt represents an integer literal.
type LiteralInt struct {
	Value int64
	Rng   Range
}

func (l *LiteralInt) nodeRange() Range { return l.Rng }
func (l *LiteralInt) exprNode()        {}

// LiteralFloat represents a floating-point literal.
type LiteralFloat struct {
	Value float64
	Rng   Range
}

func (l *LiteralFloat) nodeRange() Range { return l.Rng }
func (l *LiteralFloat) exprNode()        {}

// LiteralBool represents a boolean literal.
type LiteralBool struct {
	Value bool
	Rng   Range
}

func (l *LiteralBool) nodeRange() Range { return l.Rng }
func (l *LiteralBool) exprNode()        {}

// ListExpr represents a list expression (e.g. [1, 2, 3]).
type ListExpr struct {
	Elements []Expression
	Rng      Range
}

func (l *ListExpr) nodeRange() Range { return l.Rng }
func (l *ListExpr) exprNode()        {}

// MapExpr represents a map expression with parallel key/value slices
// to preserve insertion order.
type MapExpr struct {
	Keys   []string
	Values []Expression
	Rng    Range
}

func (m *MapExpr) nodeRange() Range { return m.Rng }
func (m *MapExpr) exprNode()        {}

// Identifier represents a simple name reference.
type Identifier struct {
	Name string
	Rng  Range
}

func (i *Identifier) nodeRange() Range { return i.Rng }
func (i *Identifier) exprNode()        {}

// Reference represents a dotted reference with 2+ segments (e.g. "db.host").
type Reference struct {
	Parts []string
	Rng   Range
}

func (r *Reference) nodeRange() Range { return r.Rng }
func (r *Reference) exprNode()        {}

// FunctionCall represents a function invocation (e.g. "env("HOME")").
type FunctionCall struct {
	Name string
	Args []Expression
	Rng  Range
}

func (f *FunctionCall) nodeRange() Range { return f.Rng }
func (f *FunctionCall) exprNode()        {}
