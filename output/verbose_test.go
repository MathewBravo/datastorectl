package output

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/provider"
)

func TestFormatPlanVerbose_create(t *testing.T) {
	res := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	res.Body.Set("name", provider.StringVal("hello"))

	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeCreate, Desired: res},
	}}

	got := FormatPlanVerbose(plan, false)

	if !strings.Contains(got, "+ svc.a (create)") {
		t.Errorf("expected create header, got:\n%s", got)
	}
	if !strings.Contains(got, `name: "hello"`) {
		t.Errorf("expected body attribute, got:\n%s", got)
	}
}

func TestFormatPlanVerbose_update_shows_bodies(t *testing.T) {
	desired := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	desired.Body.Set("timeout", provider.StringVal("14d"))
	live := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	live.Body.Set("timeout", provider.StringVal("7d"))

	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{
			ID: rid("svc", "a"), Type: engine.ChangeUpdate,
			Desired: desired, Live: live,
			Diff: engine.ResourceDiff{
				ID: rid("svc", "a"),
				Diffs: []engine.ValueDiff{
					{Kind: engine.DiffModified, Path: "timeout", Old: provider.StringVal("7d"), New: provider.StringVal("14d")},
				},
			},
		},
	}}

	got := FormatPlanVerbose(plan, false)

	if !strings.Contains(got, "diffs:") {
		t.Errorf("expected diffs section, got:\n%s", got)
	}
	if !strings.Contains(got, "desired:") {
		t.Errorf("expected desired section, got:\n%s", got)
	}
	if !strings.Contains(got, "live:") {
		t.Errorf("expected live section, got:\n%s", got)
	}
	if !strings.Contains(got, `"7d" => "14d"`) {
		t.Errorf("expected diff value, got:\n%s", got)
	}
}

func TestFormatPlanVerbose_delete_shows_live(t *testing.T) {
	live := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	live.Body.Set("name", provider.StringVal("orphan"))

	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeDelete, Live: live},
	}}

	got := FormatPlanVerbose(plan, false)

	if !strings.Contains(got, "- svc.a (delete)") {
		t.Errorf("expected delete header, got:\n%s", got)
	}
	if !strings.Contains(got, `name: "orphan"`) {
		t.Errorf("expected live body, got:\n%s", got)
	}
}

func TestFormatPlanVerbose_no_color(t *testing.T) {
	desired := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	desired.Body.Set("x", provider.StringVal("y"))
	live := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	live.Body.Set("x", provider.StringVal("z"))

	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{
			ID: rid("svc", "a"), Type: engine.ChangeUpdate,
			Desired: desired, Live: live,
			Diff: engine.ResourceDiff{
				ID:    rid("svc", "a"),
				Diffs: []engine.ValueDiff{{Kind: engine.DiffModified, Path: "x", Old: provider.StringVal("z"), New: provider.StringVal("y")}},
			},
		},
	}}

	got := FormatPlanVerbose(plan, false)

	if strings.Contains(got, "\033[") {
		t.Errorf("expected no ANSI codes when color=false, got:\n%s", got)
	}
}
