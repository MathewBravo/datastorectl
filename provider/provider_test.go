package provider

import (
	"fmt"
	"testing"
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
