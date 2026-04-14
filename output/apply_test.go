package output

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/engine"
)

func TestFormatApplyResult_success_colored(t *testing.T) {
	result := &engine.ApplyResult{
		Results: []engine.ResourceResult{
			{ID: rid("svc", "a"), Status: engine.StatusSuccess, ChangeType: engine.ChangeCreate},
			{ID: rid("svc", "b"), Status: engine.StatusSuccess, ChangeType: engine.ChangeUpdate},
			{ID: rid("svc", "c"), Status: engine.StatusSuccess, ChangeType: engine.ChangeDelete},
		},
	}

	got := FormatApplyResult(result, true)

	if !strings.Contains(got, "svc.a: created") {
		t.Errorf("expected created line, got:\n%s", got)
	}
	if !strings.Contains(got, "svc.b: updated") {
		t.Errorf("expected updated line, got:\n%s", got)
	}
	if !strings.Contains(got, "svc.c: deleted") {
		t.Errorf("expected deleted line, got:\n%s", got)
	}
	if !strings.Contains(got, ansiGreen) {
		t.Error("expected green ANSI codes for create symbol")
	}
}

func TestFormatApplyResult_failed_colored(t *testing.T) {
	result := &engine.ApplyResult{
		Results: []engine.ResourceResult{
			{ID: rid("svc", "a"), Status: engine.StatusFailed, ChangeType: engine.ChangeUpdate, Error: fmt.Errorf("connection refused")},
		},
	}

	got := FormatApplyResult(result, true)

	if !strings.Contains(got, "✕") {
		t.Error("expected ✕ symbol for failed")
	}
	if !strings.Contains(got, "connection refused") {
		t.Error("expected error message in output")
	}
	if !strings.Contains(got, ansiRed) {
		t.Error("expected red ANSI codes for failure")
	}
}

func TestFormatApplyResult_skipped_colored(t *testing.T) {
	result := &engine.ApplyResult{
		Results: []engine.ResourceResult{
			{ID: rid("svc", "a"), Status: engine.StatusSkipped, ChangeType: engine.ChangeCreate},
		},
	}

	got := FormatApplyResult(result, true)

	if !strings.Contains(got, "↓") {
		t.Error("expected ↓ symbol for skipped")
	}
	if !strings.Contains(got, ansiYellow) {
		t.Error("expected yellow ANSI codes for skipped")
	}
}

func TestFormatApplyResult_no_color(t *testing.T) {
	result := &engine.ApplyResult{
		Results: []engine.ResourceResult{
			{ID: rid("svc", "a"), Status: engine.StatusSuccess, ChangeType: engine.ChangeCreate},
			{ID: rid("svc", "b"), Status: engine.StatusFailed, ChangeType: engine.ChangeUpdate, Error: fmt.Errorf("err")},
		},
	}

	got := FormatApplyResult(result, false)

	if strings.Contains(got, "\033[") {
		t.Errorf("expected no ANSI codes when color=false, got:\n%s", got)
	}
}

func TestFormatApplyResult_includes_summary(t *testing.T) {
	result := &engine.ApplyResult{
		Results: []engine.ResourceResult{
			{ID: rid("svc", "a"), Status: engine.StatusSuccess, ChangeType: engine.ChangeCreate},
		},
	}

	got := FormatApplyResult(result, true)

	if !strings.Contains(got, "Apply complete: 1 succeeded") {
		t.Errorf("expected summary, got:\n%s", got)
	}
	if !strings.Contains(got, ansiBold) {
		t.Error("expected bold summary when colored")
	}
}
