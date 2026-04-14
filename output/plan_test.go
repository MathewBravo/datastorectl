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

func TestShouldColor_no_color_env(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// Any writer — should return false due to NO_COLOR.
	if ShouldColor(nil) {
		t.Error("expected false when NO_COLOR is set")
	}
}
