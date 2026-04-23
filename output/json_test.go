package output

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/provider"
)

func TestFormatPlanJSON_create(t *testing.T) {
	res := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
	res.Body.Set("name", provider.StringVal("hello"))

	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeCreate, Desired: res},
	}}

	data, err := FormatPlanJSON(plan)
	if err != nil {
		t.Fatalf("FormatPlanJSON failed: %v", err)
	}

	var got jsonPlan
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(got.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got.Changes))
	}
	if got.Changes[0].Action != "create" {
		t.Errorf("expected action=create, got %q", got.Changes[0].Action)
	}
	if got.Changes[0].ID.Type != "svc" || got.Changes[0].ID.Name != "a" {
		t.Errorf("unexpected ID: %+v", got.Changes[0].ID)
	}
	if got.Changes[0].Desired["name"] != "hello" {
		t.Errorf("expected desired name=hello, got %v", got.Changes[0].Desired["name"])
	}
}

func TestFormatPlanJSON_update(t *testing.T) {
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

	data, err := FormatPlanJSON(plan)
	if err != nil {
		t.Fatalf("FormatPlanJSON failed: %v", err)
	}

	var got jsonPlan
	json.Unmarshal(data, &got)

	if len(got.Changes[0].Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(got.Changes[0].Diffs))
	}
	d := got.Changes[0].Diffs[0]
	if d.Path != "timeout" || d.Kind != "modified" {
		t.Errorf("unexpected diff: %+v", d)
	}
	if d.Old != "7d" || d.New != "14d" {
		t.Errorf("expected old=7d new=14d, got old=%v new=%v", d.Old, d.New)
	}
}

func TestFormatPlanJSON_delete(t *testing.T) {
	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeDelete},
	}}

	data, err := FormatPlanJSON(plan)
	if err != nil {
		t.Fatalf("FormatPlanJSON failed: %v", err)
	}

	var got jsonPlan
	json.Unmarshal(data, &got)

	if got.Changes[0].Action != "delete" {
		t.Errorf("expected action=delete, got %q", got.Changes[0].Action)
	}
}

func TestFormatPlanJSON_no_changes(t *testing.T) {
	plan := &engine.Plan{Changes: []engine.ResourceChange{
		{ID: rid("svc", "a"), Type: engine.ChangeNoOp},
	}}

	data, err := FormatPlanJSON(plan)
	if err != nil {
		t.Fatalf("FormatPlanJSON failed: %v", err)
	}

	var got jsonPlan
	json.Unmarshal(data, &got)

	if len(got.Changes) != 0 {
		t.Errorf("expected 0 changes (no-ops omitted), got %d", len(got.Changes))
	}
	if got.Summary == "" {
		t.Error("expected summary to be present")
	}
}

func TestFormatApplyResultJSON_mixed(t *testing.T) {
	result := &engine.ApplyResult{
		Results: []engine.ResourceResult{
			{ID: rid("svc", "a"), Status: engine.StatusSuccess, ChangeType: engine.ChangeCreate},
			{ID: rid("svc", "b"), Status: engine.StatusFailed, ChangeType: engine.ChangeUpdate, Error: fmt.Errorf("timeout")},
			{ID: rid("svc", "c"), Status: engine.StatusSkipped, ChangeType: engine.ChangeCreate},
		},
	}

	data, err := FormatApplyResultJSON(result)
	if err != nil {
		t.Fatalf("FormatApplyResultJSON failed: %v", err)
	}

	var got jsonApplyResult
	json.Unmarshal(data, &got)

	if len(got.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got.Results))
	}
	if got.Results[0].Status != "success" {
		t.Errorf("expected success, got %q", got.Results[0].Status)
	}
	if got.Results[1].Status != "failed" || got.Results[1].Error != "timeout" {
		t.Errorf("expected failed with timeout, got %+v", got.Results[1])
	}
	if got.Results[2].Status != "skipped" {
		t.Errorf("expected skipped, got %q", got.Results[2].Status)
	}
}

func TestFormatPlanJSON_includes_unmanaged(t *testing.T) {
	plan := &engine.Plan{
		Changes: []engine.ResourceChange{},
		Unmanaged: []engine.ResourceChange{
			{ID: provider.ResourceID{Type: "opensearch_role", Name: "orphan"}, Type: engine.ChangeDelete},
		},
	}
	data, err := FormatPlanJSON(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Unmanaged []struct {
			ID struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"id"`
		} `json:"unmanaged"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(got.Unmanaged) != 1 {
		t.Fatalf("unmanaged len = %d, want 1", len(got.Unmanaged))
	}
	if got.Unmanaged[0].ID.Type != "opensearch_role" || got.Unmanaged[0].ID.Name != "orphan" {
		t.Errorf("Unmanaged[0].ID = %+v, want {opensearch_role, orphan}", got.Unmanaged[0].ID)
	}
	if !strings.Contains(got.Summary, "unmanaged") {
		t.Errorf("Summary missing unmanaged mention: %q", got.Summary)
	}
}

func TestFormatApplyResultJSON_summary(t *testing.T) {
	result := &engine.ApplyResult{
		Results: []engine.ResourceResult{
			{ID: rid("svc", "a"), Status: engine.StatusSuccess, ChangeType: engine.ChangeCreate},
		},
	}

	data, err := FormatApplyResultJSON(result)
	if err != nil {
		t.Fatalf("FormatApplyResultJSON failed: %v", err)
	}

	var got jsonApplyResult
	json.Unmarshal(data, &got)

	if got.Summary != "Apply complete: 1 succeeded, 0 failed, 0 skipped" {
		t.Errorf("unexpected summary: %q", got.Summary)
	}
}
