package output

import (
	"fmt"
	"strings"

	"github.com/MathewBravo/datastorectl/engine"
)

// FormatPlan renders a plan as human-readable Terraform-style text.
// When color is true, ANSI codes highlight creates (green), updates (yellow),
// and deletes (red). No-op changes are omitted.
func FormatPlan(plan *engine.Plan, color bool) string {
	var blocks []string
	for _, c := range plan.Changes {
		switch c.Type {
		case engine.ChangeCreate:
			blocks = append(blocks, formatCreate(c, color))
		case engine.ChangeUpdate:
			blocks = append(blocks, formatUpdate(c, color))
		case engine.ChangeDelete:
			blocks = append(blocks, formatDelete(c, color))
		}
	}
	if len(blocks) == 0 && len(plan.Unmanaged) == 0 {
		return "No changes."
	}
	summary := bold(plan.Summary(), color)
	if len(blocks) == 0 {
		// Only unmanaged — show the summary alone.
		return summary + "\n"
	}
	return strings.Join(blocks, "\n\n") + "\n\n" + summary + "\n"
}

func formatCreate(c engine.ResourceChange, color bool) string {
	var b strings.Builder
	header := fmt.Sprintf("+ %s (create)", c.ID)
	b.WriteString(green(header, color))
	for _, key := range c.Desired.Body.Keys() {
		v, _ := c.Desired.Body.Get(key)
		line := fmt.Sprintf("\n    %s: %s", key, v.String())
		b.WriteString(green(line, color))
	}
	return b.String()
}

func formatUpdate(c engine.ResourceChange, color bool) string {
	var b strings.Builder
	header := fmt.Sprintf("~ %s (update)", c.ID)
	b.WriteString(yellow(header, color))
	for _, d := range c.Diff.Diffs {
		b.WriteString("\n    ")
		b.WriteString(d.Path)
		b.WriteString(": ")
		b.WriteString(formatDiffValue(d, color))
	}
	return b.String()
}

func formatDelete(c engine.ResourceChange, color bool) string {
	header := fmt.Sprintf("- %s (delete)", c.ID)
	return red(header, color)
}

func formatDiffValue(d engine.ValueDiff, color bool) string {
	switch d.Kind {
	case engine.DiffModified:
		return red(d.Old.String(), color) + " => " + green(d.New.String(), color)
	case engine.DiffAdded:
		return "(added) " + green(d.New.String(), color)
	case engine.DiffRemoved:
		return "(removed) " + red(d.Old.String(), color)
	default:
		return ""
	}
}
