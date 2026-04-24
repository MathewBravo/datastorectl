package engine

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

func makeRes(typ, name string, kvs ...any) provider.Resource {
	m := provider.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i].(string), kvs[i+1].(provider.Value))
	}
	return provider.Resource{
		ID:   provider.ResourceID{Type: typ, Name: name},
		Body: m,
	}
}

func TestBuildPlan(t *testing.T) {
	t.Run("both_empty", func(t *testing.T) {
		p := BuildPlan(context.Background(), nil, nil, nil)
		if len(p.Changes) != 0 {
			t.Fatalf("expected 0 changes, got %d", len(p.Changes))
		}
		if p.HasChanges() {
			t.Fatal("expected no changes")
		}
	})

	t.Run("desired_only_creates", func(t *testing.T) {
		desired := []provider.Resource{
			makeRes("r", "a", "x", provider.IntVal(1)),
			makeRes("r", "b", "x", provider.IntVal(2)),
		}
		p := BuildPlan(context.Background(), desired, nil, nil)
		if len(p.Changes) != 2 {
			t.Fatalf("expected 2 changes, got %d", len(p.Changes))
		}
		for _, c := range p.Changes {
			if c.Type != ChangeCreate {
				t.Fatalf("expected ChangeCreate, got %s for %s", c.Type, c.ID)
			}
			if c.Desired == nil {
				t.Fatalf("expected non-nil Desired for %s", c.ID)
			}
			if c.Live != nil {
				t.Fatalf("expected nil Live for create %s", c.ID)
			}
		}
	})

	t.Run("live_only_deletes", func(t *testing.T) {
		live := []provider.Resource{
			makeRes("r", "a", "x", provider.IntVal(1)),
		}
		p := BuildPlan(context.Background(), nil, live, nil)
		if len(p.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(p.Changes))
		}
		if p.Changes[0].Type != ChangeDelete {
			t.Fatalf("expected ChangeDelete, got %s", p.Changes[0].Type)
		}
		if p.Changes[0].Live == nil {
			t.Fatal("expected non-nil Live for delete")
		}
		if p.Changes[0].Desired != nil {
			t.Fatal("expected nil Desired for delete")
		}
	})

	t.Run("identical_is_no_op", func(t *testing.T) {
		r := makeRes("r", "a", "x", provider.IntVal(1))
		desired := []provider.Resource{r}
		live := []provider.Resource{r}
		p := BuildPlan(context.Background(), desired, live, nil)
		if len(p.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(p.Changes))
		}
		if p.Changes[0].Type != ChangeNoOp {
			t.Fatalf("expected ChangeNoOp, got %s", p.Changes[0].Type)
		}
		if p.HasChanges() {
			t.Fatal("expected no actionable changes")
		}
	})

	t.Run("modified_is_update", func(t *testing.T) {
		desired := []provider.Resource{makeRes("r", "a", "x", provider.IntVal(2))}
		live := []provider.Resource{makeRes("r", "a", "x", provider.IntVal(1))}
		p := BuildPlan(context.Background(), desired, live, nil)
		if len(p.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(p.Changes))
		}
		c := p.Changes[0]
		if c.Type != ChangeUpdate {
			t.Fatalf("expected ChangeUpdate, got %s", c.Type)
		}
		if c.Desired == nil || c.Live == nil {
			t.Fatal("expected non-nil Desired and Live for update")
		}
		if !c.Diff.HasChanges() {
			t.Fatal("expected diff to have changes")
		}
	})

	t.Run("mixed_operations", func(t *testing.T) {
		desired := []provider.Resource{
			makeRes("r", "keep", "x", provider.IntVal(1)),    // no-op
			makeRes("r", "change", "x", provider.IntVal(99)), // update
			makeRes("r", "new", "x", provider.IntVal(3)),     // create
		}
		live := []provider.Resource{
			makeRes("r", "keep", "x", provider.IntVal(1)),
			makeRes("r", "change", "x", provider.IntVal(1)),
			makeRes("r", "old", "x", provider.IntVal(4)), // delete
		}
		p := BuildPlan(context.Background(), desired, live, nil)
		if len(p.Changes) != 4 {
			t.Fatalf("expected 4 changes, got %d", len(p.Changes))
		}

		// Desired-order first: keep (no-op), change (update), new (create).
		if p.Changes[0].Type != ChangeNoOp || p.Changes[0].ID.Name != "keep" {
			t.Fatalf("changes[0]: expected no-op keep, got %s %s", p.Changes[0].Type, p.Changes[0].ID)
		}
		if p.Changes[1].Type != ChangeUpdate || p.Changes[1].ID.Name != "change" {
			t.Fatalf("changes[1]: expected update change, got %s %s", p.Changes[1].Type, p.Changes[1].ID)
		}
		if p.Changes[2].Type != ChangeCreate || p.Changes[2].ID.Name != "new" {
			t.Fatalf("changes[2]: expected create new, got %s %s", p.Changes[2].Type, p.Changes[2].ID)
		}
		// Then live-only: old (delete).
		if p.Changes[3].Type != ChangeDelete || p.Changes[3].ID.Name != "old" {
			t.Fatalf("changes[3]: expected delete old, got %s %s", p.Changes[3].Type, p.Changes[3].ID)
		}

		if got := p.Summary(); got != "Plan: 1 to create, 1 to update, 1 to delete" {
			t.Fatalf("unexpected summary: %s", got)
		}
	})

	t.Run("update_carries_diff", func(t *testing.T) {
		desired := []provider.Resource{makeRes("r", "a", "x", provider.IntVal(2), "y", provider.IntVal(3))}
		live := []provider.Resource{makeRes("r", "a", "x", provider.IntVal(1))}
		p := BuildPlan(context.Background(), desired, live, nil)
		c := p.Changes[0]
		if c.Type != ChangeUpdate {
			t.Fatalf("expected ChangeUpdate, got %s", c.Type)
		}
		if len(c.Diff.Diffs) != 2 {
			t.Fatalf("expected 2 value diffs, got %d", len(c.Diff.Diffs))
		}
		// modified x, added y
		if c.Diff.Diffs[0].Kind != DiffModified || c.Diff.Diffs[0].Path != "x" {
			t.Fatalf("diffs[0]: expected DiffModified at x, got %s at %q", c.Diff.Diffs[0].Kind, c.Diff.Diffs[0].Path)
		}
		if c.Diff.Diffs[1].Kind != DiffAdded || c.Diff.Diffs[1].Path != "y" {
			t.Fatalf("diffs[1]: expected DiffAdded at y, got %s at %q", c.Diff.Diffs[1].Kind, c.Diff.Diffs[1].Path)
		}
	})

	t.Run("pointers_reference_input_slices", func(t *testing.T) {
		desired := []provider.Resource{makeRes("r", "a", "x", provider.IntVal(2))}
		live := []provider.Resource{makeRes("r", "a", "x", provider.IntVal(1))}
		p := BuildPlan(context.Background(), desired, live, nil)
		c := p.Changes[0]
		if c.Desired != &desired[0] {
			t.Fatal("Desired should point into the desired slice")
		}
		if c.Live != &live[0] {
			t.Fatal("Live should point into the live slice")
		}
	})

	t.Run("resource_differ_equal_collapses_to_noop", func(t *testing.T) {
		// Desired and live carry structurally different bodies; the
		// differ returns true so the pair becomes ChangeNoOp, and the
		// structural diff is never consulted.
		desired := []provider.Resource{makeRes("diff_r", "a", "password", provider.StringVal("cleartext"))}
		live := []provider.Resource{makeRes("diff_r", "a", "password_hash", provider.StringVal("$hash"))}
		providers := map[string]provider.Provider{
			"diff_r": &differMock{equal: true},
		}
		p := BuildPlan(context.Background(), desired, live, providers)
		if len(p.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(p.Changes))
		}
		if p.Changes[0].Type != ChangeNoOp {
			t.Errorf("expected ChangeNoOp, got %s", p.Changes[0].Type)
		}
	})

	t.Run("resource_differ_equal_false_falls_through_to_structural_diff", func(t *testing.T) {
		desired := []provider.Resource{makeRes("diff_r", "a", "x", provider.IntVal(2))}
		live := []provider.Resource{makeRes("diff_r", "a", "x", provider.IntVal(1))}
		providers := map[string]provider.Provider{
			"diff_r": &differMock{equal: false},
		}
		p := BuildPlan(context.Background(), desired, live, providers)
		if len(p.Changes) != 1 {
			t.Fatalf("expected 1 change, got %d", len(p.Changes))
		}
		if p.Changes[0].Type != ChangeUpdate {
			t.Errorf("expected ChangeUpdate from structural diff, got %s", p.Changes[0].Type)
		}
	})
}

// differMock is a provider that only implements ResourceDiffer.
// The other methods are unused by BuildPlan.
type differMock struct {
	equal bool
}

func (m *differMock) Configure(context.Context, *provider.OrderedMap) dcl.Diagnostics {
	return nil
}
func (m *differMock) Discover(context.Context) ([]provider.Resource, dcl.Diagnostics) {
	return nil, nil
}
func (m *differMock) Normalize(_ context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
	return r, nil
}
func (m *differMock) Validate(context.Context, provider.Resource) dcl.Diagnostics { return nil }
func (m *differMock) Apply(context.Context, provider.Operation, provider.Resource) dcl.Diagnostics {
	return nil
}
func (m *differMock) Schemas() map[string]provider.Schema { return nil }
func (m *differMock) Equal(_ context.Context, _, _ provider.Resource) (bool, dcl.Diagnostics) {
	return m.equal, nil
}
