package dcl

import "testing"

func TestExpressionInterfaceSatisfaction(t *testing.T) {
	rng := Range{
		Start: Pos{Filename: "test.dcl", Line: 1, Column: 1, Offset: 0},
		End:   Pos{Filename: "test.dcl", Line: 1, Column: 10, Offset: 9},
	}

	tests := []struct {
		name string
		expr Expression
	}{
		{"LiteralString", &LiteralString{Value: "hello", Rng: rng}},
		{"LiteralInt", &LiteralInt{Value: 42, Rng: rng}},
		{"LiteralFloat", &LiteralFloat{Value: 3.14, Rng: rng}},
		{"LiteralBool", &LiteralBool{Value: true, Rng: rng}},
		{"ListExpr", &ListExpr{Rng: rng}},
		{"MapExpr", &MapExpr{Rng: rng}},
		{"Identifier", &Identifier{Name: "foo", Rng: rng}},
		{"Reference", &Reference{Parts: []string{"db", "host"}, Rng: rng}},
		{"FunctionCall", &FunctionCall{Name: "env", Rng: rng}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.expr.nodeRange()
			if got != rng {
				t.Errorf("%s.nodeRange() = %v, want %v", tt.name, got, rng)
			}
		})
	}
}

func TestNodeInterfaceSatisfaction(t *testing.T) {
	rng := Range{
		Start: Pos{Filename: "test.dcl", Line: 2, Column: 1, Offset: 10},
		End:   Pos{Filename: "test.dcl", Line: 5, Column: 2, Offset: 50},
	}

	tests := []struct {
		name string
		node Node
	}{
		{"File", &File{Rng: rng}},
		{"Block", &Block{Rng: rng}},
		{"Attribute", &Attribute{Rng: rng}},
		{"LiteralString", &LiteralString{Rng: rng}},
		{"LiteralInt", &LiteralInt{Rng: rng}},
		{"LiteralFloat", &LiteralFloat{Rng: rng}},
		{"LiteralBool", &LiteralBool{Rng: rng}},
		{"ListExpr", &ListExpr{Rng: rng}},
		{"MapExpr", &MapExpr{Rng: rng}},
		{"Identifier", &Identifier{Rng: rng}},
		{"Reference", &Reference{Rng: rng}},
		{"FunctionCall", &FunctionCall{Rng: rng}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.nodeRange()
			if got != rng {
				t.Errorf("%s.nodeRange() = %v, want %v", tt.name, got, rng)
			}
		})
	}
}

func TestFileNodeRange(t *testing.T) {
	tests := []struct {
		name string
		file File
		want Range
	}{
		{
			"empty file",
			File{
				Rng: Range{
					Start: Pos{Filename: "empty.dcl", Line: 1, Column: 1, Offset: 0},
					End:   Pos{Filename: "empty.dcl", Line: 1, Column: 1, Offset: 0},
				},
			},
			Range{
				Start: Pos{Filename: "empty.dcl", Line: 1, Column: 1, Offset: 0},
				End:   Pos{Filename: "empty.dcl", Line: 1, Column: 1, Offset: 0},
			},
		},
		{
			"file with blocks",
			File{
				Blocks: []Block{
					{Type: "resource", Label: "db", Rng: Range{
						Start: Pos{Line: 1, Column: 1},
						End:   Pos{Line: 3, Column: 2},
					}},
					{Type: "resource", Label: "cache", Rng: Range{
						Start: Pos{Line: 5, Column: 1},
						End:   Pos{Line: 8, Column: 2},
					}},
				},
				Rng: Range{
					Start: Pos{Filename: "multi.dcl", Line: 1, Column: 1, Offset: 0},
					End:   Pos{Filename: "multi.dcl", Line: 8, Column: 2, Offset: 100},
				},
			},
			Range{
				Start: Pos{Filename: "multi.dcl", Line: 1, Column: 1, Offset: 0},
				End:   Pos{Filename: "multi.dcl", Line: 8, Column: 2, Offset: 100},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.file.nodeRange()
			if got != tt.want {
				t.Errorf("File.nodeRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConstructionAndRangePropagation(t *testing.T) {
	// Build a realistic AST: File → Block → Attribute → LiteralString
	strRng := Range{
		Start: Pos{Filename: "app.dcl", Line: 2, Column: 10, Offset: 25},
		End:   Pos{Filename: "app.dcl", Line: 2, Column: 22, Offset: 37},
	}
	lit := &LiteralString{Value: "localhost", Rng: strRng}

	attrRng := Range{
		Start: Pos{Filename: "app.dcl", Line: 2, Column: 3, Offset: 18},
		End:   Pos{Filename: "app.dcl", Line: 2, Column: 22, Offset: 37},
	}
	attr := Attribute{Key: "host", Value: lit, Rng: attrRng}

	blockRng := Range{
		Start: Pos{Filename: "app.dcl", Line: 1, Column: 1, Offset: 0},
		End:   Pos{Filename: "app.dcl", Line: 3, Column: 2, Offset: 40},
	}
	block := Block{
		Type:       "resource",
		Label:      "db",
		Attributes: []Attribute{attr},
		Rng:        blockRng,
	}

	fileRng := Range{
		Start: Pos{Filename: "app.dcl", Line: 1, Column: 1, Offset: 0},
		End:   Pos{Filename: "app.dcl", Line: 3, Column: 2, Offset: 40},
	}
	file := &File{
		Blocks: []Block{block},
		Rng:    fileRng,
	}

	tests := []struct {
		name string
		node Node
		want Range
	}{
		{"file range", file, fileRng},
		{"block range", &block, blockRng},
		{"attribute range", &attr, attrRng},
		{"literal string range", lit, strRng},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.nodeRange()
			if got != tt.want {
				t.Errorf("%s: nodeRange() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
