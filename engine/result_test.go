package engine

import (
	"errors"
	"testing"
)

func TestResultStatus_String(t *testing.T) {
	tests := []struct {
		status ResultStatus
		want   string
	}{
		{StatusSuccess, "success"},
		{StatusFailed, "failed"},
		{StatusSkipped, "skipped"},
		{ResultStatus(99), "ResultStatus(99)"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("ResultStatus(%d).String() = %q, want %q", int(tt.status), got, tt.want)
		}
	}
}

func TestApplyResult_HasErrors(t *testing.T) {
	t.Run("no_results", func(t *testing.T) {
		r := &ApplyResult{}
		if r.HasErrors() {
			t.Error("expected no errors for empty result")
		}
	})

	t.Run("all_success", func(t *testing.T) {
		r := &ApplyResult{Results: []ResourceResult{
			{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
			{ID: rid("r", "b"), Status: StatusSuccess, ChangeType: ChangeUpdate},
		}}
		if r.HasErrors() {
			t.Error("expected no errors when all succeeded")
		}
	})

	t.Run("has_failure", func(t *testing.T) {
		r := &ApplyResult{Results: []ResourceResult{
			{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
			{ID: rid("r", "b"), Status: StatusFailed, Error: errors.New("boom"), ChangeType: ChangeUpdate},
		}}
		if !r.HasErrors() {
			t.Error("expected errors when a resource failed")
		}
	})

	t.Run("skipped_is_not_error", func(t *testing.T) {
		r := &ApplyResult{Results: []ResourceResult{
			{ID: rid("r", "a"), Status: StatusSkipped, ChangeType: ChangeCreate},
		}}
		if r.HasErrors() {
			t.Error("skipped should not count as error")
		}
	})
}

func TestApplyResult_Failed(t *testing.T) {
	r := &ApplyResult{Results: []ResourceResult{
		{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
		{ID: rid("r", "b"), Status: StatusFailed, Error: errors.New("b failed"), ChangeType: ChangeUpdate},
		{ID: rid("r", "c"), Status: StatusSkipped, ChangeType: ChangeCreate},
		{ID: rid("r", "d"), Status: StatusFailed, Error: errors.New("d failed"), ChangeType: ChangeDelete},
	}}

	failed := r.Failed()
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed, got %d", len(failed))
	}
	if failed[0].ID != rid("r", "b") {
		t.Errorf("failed[0]: expected r.b, got %v", failed[0].ID)
	}
	if failed[1].ID != rid("r", "d") {
		t.Errorf("failed[1]: expected r.d, got %v", failed[1].ID)
	}
}

func TestApplyResult_Skipped(t *testing.T) {
	r := &ApplyResult{Results: []ResourceResult{
		{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
		{ID: rid("r", "b"), Status: StatusSkipped, ChangeType: ChangeCreate},
		{ID: rid("r", "c"), Status: StatusFailed, Error: errors.New("fail"), ChangeType: ChangeDelete},
		{ID: rid("r", "d"), Status: StatusSkipped, ChangeType: ChangeUpdate},
	}}

	skipped := r.Skipped()
	if len(skipped) != 2 {
		t.Fatalf("expected 2 skipped, got %d", len(skipped))
	}
	if skipped[0].ID != rid("r", "b") {
		t.Errorf("skipped[0]: expected r.b, got %v", skipped[0].ID)
	}
	if skipped[1].ID != rid("r", "d") {
		t.Errorf("skipped[1]: expected r.d, got %v", skipped[1].ID)
	}
}

func TestApplyResult_Summary(t *testing.T) {
	t.Run("mixed_results", func(t *testing.T) {
		r := &ApplyResult{Results: []ResourceResult{
			{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
			{ID: rid("r", "b"), Status: StatusSuccess, ChangeType: ChangeUpdate},
			{ID: rid("r", "c"), Status: StatusFailed, Error: errors.New("fail"), ChangeType: ChangeDelete},
			{ID: rid("r", "d"), Status: StatusSkipped, ChangeType: ChangeCreate},
		}}
		want := "Apply complete: 2 succeeded, 1 failed, 1 skipped"
		if got := r.Summary(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty_result", func(t *testing.T) {
		r := &ApplyResult{}
		want := "Apply complete: 0 succeeded, 0 failed, 0 skipped"
		if got := r.Summary(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("all_success", func(t *testing.T) {
		r := &ApplyResult{Results: []ResourceResult{
			{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
			{ID: rid("r", "b"), Status: StatusSuccess, ChangeType: ChangeCreate},
			{ID: rid("r", "c"), Status: StatusSuccess, ChangeType: ChangeCreate},
		}}
		want := "Apply complete: 3 succeeded, 0 failed, 0 skipped"
		if got := r.Summary(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestApplyResult_FilterEdgeCases(t *testing.T) {
	t.Run("failed_returns_nil_when_none", func(t *testing.T) {
		r := &ApplyResult{Results: []ResourceResult{
			{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
		}}
		if got := r.Failed(); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("skipped_returns_nil_when_none", func(t *testing.T) {
		r := &ApplyResult{Results: []ResourceResult{
			{ID: rid("r", "a"), Status: StatusSuccess, ChangeType: ChangeCreate},
		}}
		if got := r.Skipped(); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestResourceResult_PreservesError(t *testing.T) {
	origErr := errors.New("connection refused")
	res := ResourceResult{
		ID:         rid("db", "main"),
		Status:     StatusFailed,
		Error:      origErr,
		ChangeType: ChangeCreate,
	}
	if !errors.Is(res.Error, origErr) {
		t.Error("expected original error to be preserved")
	}
}
