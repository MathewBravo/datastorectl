package provider

import (
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
)

func TestResourceIDString(t *testing.T) {
	tests := []struct {
		name string
		id   ResourceID
		want string
	}{
		{
			name: "standard",
			id:   ResourceID{Type: "opensearch_ism_policy", Name: "hot_warm_delete"},
			want: "opensearch_ism_policy.hot_warm_delete",
		},
		{
			name: "empty type",
			id:   ResourceID{Type: "", Name: "hot_warm_delete"},
			want: ".hot_warm_delete",
		},
		{
			name: "empty name",
			id:   ResourceID{Type: "opensearch_ism_policy", Name: ""},
			want: "opensearch_ism_policy.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.id.String()
			if got != tt.want {
				t.Errorf("ResourceID.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceZeroSourceRange(t *testing.T) {
	r := Resource{
		ID:   ResourceID{Type: "s3_bucket", Name: "logs"},
		Body: NewOrderedMap(),
	}
	zero := dcl.Range{}
	if r.SourceRange != zero {
		t.Errorf("expected zero-value SourceRange, got %v", r.SourceRange)
	}
}

func TestResourceBody(t *testing.T) {
	body := NewOrderedMap()
	body.Set("policy_id", StringVal("hot_warm_delete"))
	body.Set("min_index_age", IntVal(30))

	r := Resource{
		ID:   ResourceID{Type: "opensearch_ism_policy", Name: "hot_warm_delete"},
		Body: body,
	}

	v, ok := r.Body.Get("policy_id")
	if !ok {
		t.Fatal("expected key policy_id to exist")
	}
	if v.Str != "hot_warm_delete" {
		t.Errorf("policy_id = %q, want %q", v.Str, "hot_warm_delete")
	}

	v, ok = r.Body.Get("min_index_age")
	if !ok {
		t.Fatal("expected key min_index_age to exist")
	}
	if v.Int != 30 {
		t.Errorf("min_index_age = %d, want 30", v.Int)
	}

	if r.Body.Len() != 2 {
		t.Errorf("Body.Len() = %d, want 2", r.Body.Len())
	}
}
