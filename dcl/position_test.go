package dcl

import "testing"

func TestPosString(t *testing.T) {
	tests := []struct {
		name string
		pos  Pos
		want string
	}{
		{"with filename", Pos{Filename: "main.dcl", Line: 1, Column: 5}, "main.dcl:1:5"},
		{"without filename", Pos{Line: 3, Column: 10}, "3:10"},
		{"zero values with filename", Pos{Filename: "a.dcl"}, "a.dcl:0:0"},
		{"zero values without filename", Pos{}, "0:0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pos.String(); got != tt.want {
				t.Errorf("Pos.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRangeString(t *testing.T) {
	tests := []struct {
		name string
		r    Range
		want string
	}{
		{
			"with filename",
			Range{Pos{"f.dcl", 1, 1, 0}, Pos{"f.dcl", 1, 10, 9}},
			"f.dcl:1:1-1:10",
		},
		{
			"without filename",
			Range{Pos{"", 1, 1, 0}, Pos{"", 2, 5, 20}},
			"1:1-2:5",
		},
		{
			"single character",
			Range{Pos{"x.dcl", 5, 3, 40}, Pos{"x.dcl", 5, 4, 41}},
			"x.dcl:5:3-5:4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("Range.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
