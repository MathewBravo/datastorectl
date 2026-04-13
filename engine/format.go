package engine

import (
	"fmt"
	"strings"
)

// FormatPlan returns a human-readable Terraform-style representation of a plan.
// No-op changes are omitted. Plain text only — no ANSI color codes.
func FormatPlan(plan *Plan) string {
	var blocks []string
	for _, c := range plan.Changes {
		switch c.Type {
		case ChangeCreate:
			blocks = append(blocks, formatCreate(c))
		case ChangeUpdate:
			blocks = append(blocks, formatUpdate(c))
		case ChangeDelete:
			blocks = append(blocks, formatDelete(c))
		}
	}
	if len(blocks) == 0 {
		return "No changes."
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

func formatCreate(c ResourceChange) string {
	var b strings.Builder
	fmt.Fprintf(&b, "+ %s (create)", c.ID)
	for _, key := range c.Desired.Body.Keys() {
		v, _ := c.Desired.Body.Get(key)
		fmt.Fprintf(&b, "\n    %s: %s", key, v.String())
	}
	return b.String()
}

func formatUpdate(c ResourceChange) string {
	var b strings.Builder
	fmt.Fprintf(&b, "~ %s (update)", c.ID)
	for _, d := range c.Diff.Diffs {
		fmt.Fprintf(&b, "\n    %s: %s", d.Path, formatDiffValue(d))
	}
	return b.String()
}

func formatDelete(c ResourceChange) string {
	return fmt.Sprintf("- %s (delete)", c.ID)
}

func formatDiffValue(d ValueDiff) string {
	switch d.Kind {
	case DiffModified:
		return fmt.Sprintf("%s => %s", d.Old.String(), d.New.String())
	case DiffAdded:
		return fmt.Sprintf("(added) %s", d.New.String())
	case DiffRemoved:
		return fmt.Sprintf("(removed) %s", d.Old.String())
	default:
		return ""
	}
}

// FormatApplyResult returns a human-readable summary of apply outcomes.
func FormatApplyResult(result *ApplyResult) string {
	var b strings.Builder
	for i, r := range result.Results {
		if i > 0 {
			b.WriteByte('\n')
		}
		symbol := applySymbol(r)
		fmt.Fprintf(&b, "%s %s: %s", symbol, r.ID, applyOutcome(r))
	}
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(result.Summary())
	return b.String()
}

func applySymbol(r ResourceResult) string {
	switch r.Status {
	case StatusFailed:
		return "✕"
	case StatusSkipped:
		return "↓"
	default:
		switch r.ChangeType {
		case ChangeCreate:
			return "+"
		case ChangeUpdate:
			return "~"
		case ChangeDelete:
			return "-"
		default:
			return " "
		}
	}
}

func applyOutcome(r ResourceResult) string {
	switch r.Status {
	case StatusSuccess:
		return r.ChangeType.String() + "d"
	case StatusFailed:
		return fmt.Sprintf("failed (%s)", r.Error.Error())
	case StatusSkipped:
		return "skipped"
	default:
		return r.Status.String()
	}
}

