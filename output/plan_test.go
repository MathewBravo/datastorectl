package output

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/provider"
)

func rid(typ, name string) provider.ResourceID {
	return provider.ResourceID{Type: typ, Name: name}
}

func TestFormatPlan_create_colored(t *testing.T) {
	res := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	res.Body.Set("name", provider.StringVal("hello"))

	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeCreate, Desired: res},
	}}

	got := FormatPlan(plan, true)

	if !strings.Contains(got, ansiGreen) {
		t.Error("expected green ANSI codes for create")
	}
	if !strings.Contains(got, "+ svc.a (create)") {
		t.Errorf("expected create header, got:\n%s", got)
	}
}

func TestFormatPlan_update_colored(t *testing.T) {
	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{
			ID:   rid("svc", "a"),
			Type: engine.ChangeUpdate,
			Diff: engine.ResourceDiff{
				ID: rid("svc", "a"),
				Diffs: []engine.ValueDiff{
					{Kind: engine.DiffModified, Path: "timeout", Old: provider.StringVal("7d"), New: provider.StringVal("14d")},
				},
			},
		},
	}}

	got := FormatPlan(plan, true)

	if !strings.Contains(got, ansiYellow) {
		t.Error("expected yellow ANSI codes for update header")
	}
	if !strings.Contains(got, ansiRed) {
		t.Error("expected red ANSI codes for old value")
	}
	if !strings.Contains(got, ansiGreen) {
		t.Error("expected green ANSI codes for new value")
	}
}

func TestFormatPlan_delete_colored(t *testing.T) {
	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeDelete},
	}}

	got := FormatPlan(plan, true)

	if !strings.Contains(got, ansiRed) {
		t.Error("expected red ANSI codes for delete")
	}
	if !strings.Contains(got, "- svc.a (delete)") {
		t.Errorf("expected delete header, got:\n%s", got)
	}
}

func TestFormatPlan_no_color(t *testing.T) {
	res := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	res.Body.Set("name", provider.StringVal("hello"))

	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeCreate, Desired: res},
		{ID: rid("svc", "b"), Type: engine.ChangeDelete},
	}}

	got := FormatPlan(plan, false)

	if strings.Contains(got, "\033[") {
		t.Errorf("expected no ANSI codes when color=false, got:\n%s", got)
	}
	if !strings.Contains(got, "+ svc.a (create)") {
		t.Error("expected create header")
	}
	if !strings.Contains(got, "- svc.b (delete)") {
		t.Error("expected delete header")
	}
}

func TestFormatPlan_no_changes(t *testing.T) {
	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeNoOp},
	}}

	got := FormatPlan(plan, true)

	if got != "No changes." {
		t.Errorf("expected 'No changes.', got: %s", got)
	}
}

func TestFormatPlan_includes_summary(t *testing.T) {
	res := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeCreate, Desired: res},
	}}

	got := FormatPlan(plan, true)

	if !strings.Contains(got, "Plan: 1 to create") {
		t.Errorf("expected summary line, got:\n%s", got)
	}
	if !strings.Contains(got, ansiBold) {
		t.Error("expected bold summary when colored")
	}
}

func TestFormatPlan_unmanaged_only(t *testing.T) {
	plan := &engine.Plan{
		Unmanaged: []engine.ResourceChange{
			{ID: provider.ResourceID{Type: "t", Name: "x"}, Type: engine.ChangeDelete},
			{ID: provider.ResourceID{Type: "t", Name: "y"}, Type: engine.ChangeDelete},
		},
	}
	got := FormatPlan(plan, false)
	// Must surface the unmanaged count; must NOT render per-resource delete lines.
	if !strings.Contains(got, "2 unmanaged resources") {
		t.Errorf("FormatPlan missing unmanaged count: %q", got)
	}
	if strings.Contains(got, "(delete)") {
		t.Errorf("FormatPlan listed deletes per-resource when it shouldn't: %q", got)
	}
	if got == "No changes." {
		t.Errorf("FormatPlan returned 'No changes.' but there are unmanaged resources")
	}
}

func TestFormatPlan_creates_with_unmanaged(t *testing.T) {
	desired := provider.NewOrderedMap()
	desired.Set("name", provider.StringVal("hello"))
	plan := &engine.Plan{
		Changes: []engine.ResourceChange{
			{
				ID: provider.ResourceID{Type: "t", Name: "new"}, Type: engine.ChangeCreate,
				Desired: &provider.Resource{ID: provider.ResourceID{Type: "t", Name: "new"}, Body: desired},
			},
		},
		Unmanaged: []engine.ResourceChange{
			{ID: provider.ResourceID{Type: "t", Name: "orphan"}, Type: engine.ChangeDelete},
		},
	}
	got := FormatPlan(plan, false)
	if !strings.Contains(got, "+ t.new (create)") {
		t.Errorf("FormatPlan missing create line: %q", got)
	}
	if !strings.Contains(got, "1 unmanaged resources") {
		t.Errorf("FormatPlan missing unmanaged count: %q", got)
	}
}

func TestShouldColor_no_color_env(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// Any writer — should return false due to NO_COLOR.
	if ShouldColor(nil) {
		t.Error("expected false when NO_COLOR is set")
	}
}

func TestFormatPlan_marks_guarded_deletes(t *testing.T) {
	plan := &engine.Plan{
		Changes: []engine.ResourceChange{{
			ID:   rid("opensearch_role_mapping", "all_access"),
			Type: engine.ChangeDelete,
			Live: &provider.Resource{ID: rid("opensearch_role_mapping", "all_access")},
		}},
		Guards: []engine.Guard{{
			Resource: rid("opensearch_role_mapping", "all_access"),
			Reason:   `would revoke caller "admin" access`,
		}},
	}

	out := FormatPlan(plan, false)
	if !strings.Contains(out, "(would lock out caller)") {
		t.Errorf("output missing lockout marker: %s", out)
	}
	if !strings.Contains(out, `would revoke caller "admin" access`) {
		t.Errorf("output missing guard reason: %s", out)
	}
}

func TestFormatPlan_unguarded_delete_has_no_marker(t *testing.T) {
	plan := &engine.Plan{
		Changes: []engine.ResourceChange{{
			ID:   rid("svc", "deleteme"),
			Type: engine.ChangeDelete,
			Live: &provider.Resource{ID: rid("svc", "deleteme")},
		}},
	}

	out := FormatPlan(plan, false)
	if strings.Contains(out, "(would lock out caller)") {
		t.Errorf("unguarded delete should not have lockout marker: %s", out)
	}
}

func TestFormatPlanVerbose_marks_guarded_deletes(t *testing.T) {
	plan := &engine.Plan{
		Changes: []engine.ResourceChange{{
			ID:   rid("opensearch_internal_user", "admin"),
			Type: engine.ChangeDelete,
			Live: &provider.Resource{ID: rid("opensearch_internal_user", "admin"), Body: provider.NewOrderedMap()},
		}},
		Guards: []engine.Guard{{
			Resource: rid("opensearch_internal_user", "admin"),
			Reason:   "would delete caller's own account",
		}},
	}
	out := FormatPlanVerbose(plan, false)
	if !strings.Contains(out, "(would lock out caller)") {
		t.Errorf("verbose output missing lockout marker: %s", out)
	}
	if !strings.Contains(out, "would delete caller's own account") {
		t.Errorf("verbose output missing guard reason: %s", out)
	}
}
