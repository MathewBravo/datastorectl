package output

import (
	"fmt"
	"strings"

	"github.com/MathewBravo/datastorectl/engine"
)

// FormatApplyResult renders apply outcomes as human-readable text.
// When color is true, ANSI codes highlight success (green), failure (red),
// and skipped (yellow) results.
func FormatApplyResult(result *engine.ApplyResult, color bool) string {
	var b strings.Builder
	for i, r := range result.Results {
		if i > 0 {
			b.WriteByte('\n')
		}
		symbol := applySymbol(r, color)
		outcome := applyOutcome(r, color)
		fmt.Fprintf(&b, "%s %s: %s", symbol, r.ID, outcome)
	}
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(bold(result.Summary(), color))
	return b.String()
}

func applySymbol(r engine.ResourceResult, color bool) string {
	switch r.Status {
	case engine.StatusFailed:
		return boldRed("✕", color)
	case engine.StatusSkipped:
		return yellow("↓", color)
	default:
		switch r.ChangeType {
		case engine.ChangeCreate:
			return green("+", color)
		case engine.ChangeUpdate:
			return yellow("~", color)
		case engine.ChangeDelete:
			return red("-", color)
		default:
			return " "
		}
	}
}

func applyOutcome(r engine.ResourceResult, color bool) string {
	switch r.Status {
	case engine.StatusSuccess:
		return r.ChangeType.String() + "d"
	case engine.StatusFailed:
		msg := fmt.Sprintf("failed (%s)", r.Error.Error())
		return red(msg, color)
	case engine.StatusSkipped:
		return yellow("skipped", color)
	default:
		return r.Status.String()
	}
}
