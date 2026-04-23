package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
)

// Compile-time check that Operation implements fmt.Stringer.
var _ fmt.Stringer = Operation(0)

func TestOperationString(t *testing.T) {
	tests := []struct {
		op   Operation
		want string
	}{
		{OpCreate, "create"},
		{OpUpdate, "update"},
		{OpDelete, "delete"},
		{Operation(99), "Operation(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.op.String(); got != tt.want {
				t.Errorf("Operation(%d).String() = %q, want %q", int(tt.op), got, tt.want)
			}
		})
	}
}

type stubTypeOrderer struct{ stubProvider }

func (stubTypeOrderer) TypeOrderings() []TypeOrdering {
	return []TypeOrdering{{Before: "role", After: "role_mapping"}}
}

var _ TypeOrderer = stubTypeOrderer{}

func TestTypeOrdererOptional(t *testing.T) {
	var p Provider = stubTypeOrderer{}
	to, ok := p.(TypeOrderer)
	if !ok {
		t.Fatal("expected stubTypeOrderer to satisfy TypeOrderer")
	}
	orderings := to.TypeOrderings()
	if len(orderings) != 1 || orderings[0].Before != "role" {
		t.Errorf("unexpected orderings: %v", orderings)
	}

	var plain Provider = stubProvider{}
	if _, ok := plain.(TypeOrderer); ok {
		t.Error("stubProvider should not satisfy TypeOrderer")
	}
}

type stubGuarder struct{ stubProvider }

func (stubGuarder) GuardDeletes(context.Context, []Resource) ([]DeleteGuard, dcl.Diagnostics) {
	return nil, nil
}

var _ DeleteGuarder = stubGuarder{}

func TestDeleteGuarderOptional(t *testing.T) {
	var p Provider = stubGuarder{}
	if _, ok := p.(DeleteGuarder); !ok {
		t.Fatal("expected stubGuarder to satisfy DeleteGuarder")
	}
	var plain Provider = stubProvider{}
	if _, ok := plain.(DeleteGuarder); ok {
		t.Error("stubProvider should not satisfy DeleteGuarder")
	}
}

func TestOperationConstants(t *testing.T) {
	tests := []struct {
		op   Operation
		want int
	}{
		{OpCreate, 0},
		{OpUpdate, 1},
		{OpDelete, 2},
	}
	for _, tt := range tests {
		t.Run(tt.op.String(), func(t *testing.T) {
			if got := int(tt.op); got != tt.want {
				t.Errorf("int(%s) = %d, want %d", tt.op, got, tt.want)
			}
		})
	}
}
