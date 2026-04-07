// position.go defines source-location types used across the DCL parser pipeline.
package dcl

import "fmt"

// Pos represents a single position in a source file.
type Pos struct {
	Filename string
	Line     int
	Column   int
	Offset   int
}

func (p Pos) String() string {
	if p.Filename != "" {
		return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Range represents a span of source code between two positions.
type Range struct {
	Start Pos
	End   Pos
}

func (r Range) String() string {
	if r.Start.Filename != "" {
		return fmt.Sprintf("%s:%d:%d-%d:%d", r.Start.Filename, r.Start.Line, r.Start.Column, r.End.Line, r.End.Column)
	}
	return fmt.Sprintf("%d:%d-%d:%d", r.Start.Line, r.Start.Column, r.End.Line, r.End.Column)
}
