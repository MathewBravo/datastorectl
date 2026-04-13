package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestFormatPlan(t *testing.T) {
	t.Run("format_create", func(t *testing.T) {
		res := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
		res.Body.Set("name", provider.StringVal("hello"))
		res.Body.Set("count", provider.IntVal(3))

		plan := &Plan{
			Changes: []ResourceChange{
				{ID: rid("svc", "a"), Type: ChangeCreate, Desired: res},
			},
		}

		got := FormatPlan(plan)

		if !strings.Contains(got, "+ svc.a (create)") {
			t.Errorf("expected create header, got:\n%s", got)
		}
		if !strings.Contains(got, `    name: "hello"`) {
			t.Errorf("expected name attr, got:\n%s", got)
		}
		if !strings.Contains(got, "    count: 3") {
			t.Errorf("expected count attr, got:\n%s", got)
		}
	})

	t.Run("format_update", func(t *testing.T) {
		plan := &Plan{
			Changes: []ResourceChange{
				{
					ID:   rid("svc", "a"),
					Type: ChangeUpdate,
					Diff: ResourceDiff{
						ID: rid("svc", "a"),
						Diffs: []ValueDiff{
							{Kind: DiffModified, Path: "timeout", Old: provider.StringVal("7d"), New: provider.StringVal("14d")},
							{Kind: DiffAdded, Path: "retries", New: provider.IntVal(3)},
							{Kind: DiffRemoved, Path: "legacy", Old: provider.StringVal("old")},
						},
					},
				},
			},
		}

		got := FormatPlan(plan)

		if !strings.Contains(got, "~ svc.a (update)") {
			t.Errorf("expected update header, got:\n%s", got)
		}
		if !strings.Contains(got, `    timeout: "7d" => "14d"`) {
			t.Errorf("expected modified diff, got:\n%s", got)
		}
		if !strings.Contains(got, "    retries: (added) 3") {
			t.Errorf("expected added diff, got:\n%s", got)
		}
		if !strings.Contains(got, `    legacy: (removed) "old"`) {
			t.Errorf("expected removed diff, got:\n%s", got)
		}
	})

	t.Run("format_delete", func(t *testing.T) {
		plan := &Plan{
			Changes: []ResourceChange{
				{ID: rid("svc", "a"), Type: ChangeDelete},
			},
		}

		got := FormatPlan(plan)

		if !strings.Contains(got, "- svc.a (delete)") {
			t.Errorf("expected delete header, got:\n%s", got)
		}
		// Should not contain indented attributes.
		lines := strings.Split(got, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "    ") {
				t.Errorf("delete should not have body attrs, found: %s", line)
			}
		}
	})

	t.Run("format_no_changes", func(t *testing.T) {
		plan := &Plan{
			Changes: []ResourceChange{
				{ID: rid("svc", "a"), Type: ChangeNoOp},
			},
		}

		got := FormatPlan(plan)

		if got != "No changes." {
			t.Errorf("expected 'No changes.', got: %s", got)
		}
	})

	t.Run("format_mixed", func(t *testing.T) {
		created := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
		created.Body.Set("key", provider.StringVal("val"))

		plan := &Plan{
			Changes: []ResourceChange{
				{ID: rid("svc", "a"), Type: ChangeCreate, Desired: created},
				{
					ID:   rid("svc", "b"),
					Type: ChangeUpdate,
					Diff: ResourceDiff{
						ID:    rid("svc", "b"),
						Diffs: []ValueDiff{{Kind: DiffModified, Path: "x", Old: provider.IntVal(1), New: provider.IntVal(2)}},
					},
				},
				{ID: rid("svc", "c"), Type: ChangeDelete},
				{ID: rid("svc", "d"), Type: ChangeNoOp},
			},
		}

		got := FormatPlan(plan)

		if !strings.Contains(got, "+ svc.a (create)") {
			t.Errorf("expected create block, got:\n%s", got)
		}
		if !strings.Contains(got, "~ svc.b (update)") {
			t.Errorf("expected update block, got:\n%s", got)
		}
		if !strings.Contains(got, "- svc.c (delete)") {
			t.Errorf("expected delete block, got:\n%s", got)
		}
		// No-op should not appear.
		if strings.Contains(got, "svc.d") {
			t.Errorf("no-op should not appear in output, got:\n%s", got)
		}
	})
}

func TestFormatApplyResult(t *testing.T) {
	t.Run("format_apply_success", func(t *testing.T) {
		result := &ApplyResult{
			Results: []ResourceResult{
				{ID: rid("svc", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
				{ID: rid("svc", "b"), Status: StatusSuccess, ChangeType: ChangeUpdate},
				{ID: rid("svc", "c"), Status: StatusSuccess, ChangeType: ChangeDelete},
			},
		}

		got := FormatApplyResult(result)

		if !strings.Contains(got, "+ svc.a: created") {
			t.Errorf("expected created line, got:\n%s", got)
		}
		if !strings.Contains(got, "~ svc.b: updated") {
			t.Errorf("expected updated line, got:\n%s", got)
		}
		if !strings.Contains(got, "- svc.c: deleted") {
			t.Errorf("expected deleted line, got:\n%s", got)
		}
		if !strings.Contains(got, "Apply complete: 3 succeeded, 0 failed, 0 skipped") {
			t.Errorf("expected summary line, got:\n%s", got)
		}
	})

	t.Run("format_apply_mixed", func(t *testing.T) {
		result := &ApplyResult{
			Results: []ResourceResult{
				{ID: rid("svc", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
				{ID: rid("svc", "b"), Status: StatusFailed, ChangeType: ChangeUpdate, Error: fmt.Errorf("connection refused")},
				{ID: rid("svc", "c"), Status: StatusSkipped, ChangeType: ChangeCreate},
			},
		}

		got := FormatApplyResult(result)

		if !strings.Contains(got, "+ svc.a: created") {
			t.Errorf("expected created line, got:\n%s", got)
		}
		if !strings.Contains(got, "✕ svc.b: failed (connection refused)") {
			t.Errorf("expected failed line with error, got:\n%s", got)
		}
		if !strings.Contains(got, "↓ svc.c: skipped") {
			t.Errorf("expected skipped line, got:\n%s", got)
		}
		if !strings.Contains(got, "Apply complete: 1 succeeded, 1 failed, 1 skipped") {
			t.Errorf("expected summary line, got:\n%s", got)
		}
	})
}
