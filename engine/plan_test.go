package engine

import (
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestChangeType_String(t *testing.T) {
	tests := []struct {
		ct   ChangeType
		want string
	}{
		{ChangeNoOp, "no-op"},
		{ChangeCreate, "create"},
		{ChangeUpdate, "update"},
		{ChangeDelete, "delete"},
		{ChangeType(99), "ChangeType(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.ct.String(); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestPlan_HasChanges(t *testing.T) {
	t.Run("empty_plan", func(t *testing.T) {
		p := Plan{}
		if p.HasChanges() {
			t.Fatal("expected no changes for empty plan")
		}
	})

	t.Run("only_no_ops", func(t *testing.T) {
		p := Plan{Changes: []ResourceChange{
			{Type: ChangeNoOp},
			{Type: ChangeNoOp},
		}}
		if p.HasChanges() {
			t.Fatal("expected no changes for no-op only plan")
		}
	})

	t.Run("has_create", func(t *testing.T) {
		p := Plan{Changes: []ResourceChange{
			{Type: ChangeNoOp},
			{Type: ChangeCreate},
		}}
		if !p.HasChanges() {
			t.Fatal("expected changes when plan has a create")
		}
	})

	t.Run("has_update", func(t *testing.T) {
		p := Plan{Changes: []ResourceChange{{Type: ChangeUpdate}}}
		if !p.HasChanges() {
			t.Fatal("expected changes when plan has an update")
		}
	})

	t.Run("has_delete", func(t *testing.T) {
		p := Plan{Changes: []ResourceChange{{Type: ChangeDelete}}}
		if !p.HasChanges() {
			t.Fatal("expected changes when plan has a delete")
		}
	})

	t.Run("excludes_unmanaged", func(t *testing.T) {
		// Only unmanaged deletes → HasChanges must be false (drives exit code 0).
		p := Plan{Unmanaged: []ResourceChange{
			{ID: provider.ResourceID{Type: "t", Name: "x"}, Type: ChangeDelete},
		}}
		if p.HasChanges() {
			t.Fatal("HasChanges() = true, want false (unmanaged shouldn't count)")
		}
	})
}

func TestPlan_Filters(t *testing.T) {
	rid := func(typ, name string) provider.ResourceID {
		return provider.ResourceID{Type: typ, Name: name}
	}

	p := Plan{Changes: []ResourceChange{
		{ID: rid("a", "1"), Type: ChangeCreate},
		{ID: rid("a", "2"), Type: ChangeUpdate},
		{ID: rid("a", "3"), Type: ChangeDelete},
		{ID: rid("a", "4"), Type: ChangeNoOp},
		{ID: rid("b", "1"), Type: ChangeCreate},
		{ID: rid("b", "2"), Type: ChangeUpdate},
	}}

	t.Run("creates", func(t *testing.T) {
		got := p.Creates()
		if len(got) != 2 {
			t.Fatalf("expected 2 creates, got %d", len(got))
		}
		if got[0].ID.Name != "1" || got[1].ID.Name != "1" {
			t.Fatalf("unexpected creates: %v", got)
		}
	})

	t.Run("updates", func(t *testing.T) {
		got := p.Updates()
		if len(got) != 2 {
			t.Fatalf("expected 2 updates, got %d", len(got))
		}
	})

	t.Run("deletes", func(t *testing.T) {
		got := p.Deletes()
		if len(got) != 1 {
			t.Fatalf("expected 1 delete, got %d", len(got))
		}
		if got[0].ID != rid("a", "3") {
			t.Fatalf("expected a.3, got %s", got[0].ID)
		}
	})

	t.Run("empty_filter", func(t *testing.T) {
		empty := Plan{}
		if got := empty.Creates(); got != nil {
			t.Fatalf("expected nil creates, got %v", got)
		}
	})
}

func TestPlan_Summary(t *testing.T) {
	t.Run("mixed_changes", func(t *testing.T) {
		p := Plan{Changes: []ResourceChange{
			{Type: ChangeCreate},
			{Type: ChangeCreate},
			{Type: ChangeUpdate},
			{Type: ChangeDelete},
			{Type: ChangeNoOp},
		}}
		want := "Plan: 2 to create, 1 to update, 1 to delete"
		if got := p.Summary(); got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("no_changes", func(t *testing.T) {
		p := Plan{}
		want := "Plan: 0 to create, 0 to update"
		if got := p.Summary(); got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("creates_and_updates_only", func(t *testing.T) {
		p := Plan{Changes: []ResourceChange{
			{ID: provider.ResourceID{Type: "t", Name: "a"}, Type: ChangeCreate},
			{ID: provider.ResourceID{Type: "t", Name: "b"}, Type: ChangeCreate},
			{ID: provider.ResourceID{Type: "t", Name: "c"}, Type: ChangeUpdate},
		}}
		want := "Plan: 2 to create, 1 to update"
		if got := p.Summary(); got != want {
			t.Fatalf("Summary() = %q, want %q", got, want)
		}
	})

	t.Run("with_unmanaged", func(t *testing.T) {
		p := Plan{
			Changes: []ResourceChange{
				{ID: provider.ResourceID{Type: "t", Name: "a"}, Type: ChangeCreate},
			},
			Unmanaged: []ResourceChange{
				{ID: provider.ResourceID{Type: "t", Name: "x"}, Type: ChangeDelete},
				{ID: provider.ResourceID{Type: "t", Name: "y"}, Type: ChangeDelete},
			},
		}
		want := "Plan: 1 to create, 0 to update (2 unmanaged resources — use --prune to delete)"
		if got := p.Summary(); got != want {
			t.Fatalf("Summary() = %q, want %q", got, want)
		}
	})

	t.Run("prune_mode", func(t *testing.T) {
		// When Prune is on, deletes live inside Changes, not Unmanaged.
		p := Plan{Changes: []ResourceChange{
			{ID: provider.ResourceID{Type: "t", Name: "a"}, Type: ChangeCreate},
			{ID: provider.ResourceID{Type: "t", Name: "x"}, Type: ChangeDelete},
			{ID: provider.ResourceID{Type: "t", Name: "y"}, Type: ChangeDelete},
		}}
		want := "Plan: 1 to create, 0 to update, 2 to delete"
		if got := p.Summary(); got != want {
			t.Fatalf("Summary() = %q, want %q", got, want)
		}
	})
}

func TestResourceChange_Fields(t *testing.T) {
	body := provider.NewOrderedMap()
	body.Set("a", provider.IntVal(1))
	desired := provider.Resource{
		ID:   provider.ResourceID{Type: "r", Name: "x"},
		Body: body,
	}
	diff := ResourceDiff{
		ID:    desired.ID,
		Diffs: []ValueDiff{{Kind: DiffAdded, Path: "a"}},
	}
	rc := ResourceChange{
		ID:      desired.ID,
		Type:    ChangeCreate,
		Desired: &desired,
		Live:    nil,
		Diff:    diff,
	}

	if rc.ID.String() != "r.x" {
		t.Fatalf("expected ID r.x, got %s", rc.ID)
	}
	if rc.Type != ChangeCreate {
		t.Fatalf("expected ChangeCreate, got %s", rc.Type)
	}
	if rc.Desired == nil {
		t.Fatal("expected non-nil Desired")
	}
	if rc.Live != nil {
		t.Fatal("expected nil Live for create")
	}
	if len(rc.Diff.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(rc.Diff.Diffs))
	}
}
