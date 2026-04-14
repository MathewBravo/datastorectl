package output

import (
	"fmt"
	"strings"

	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/provider"
)

// FormatPlanVerbose renders a plan with full before/after bodies for each change.
// Creates show the full desired body. Updates show diffs, desired, and live bodies.
// Deletes show the full live body.
func FormatPlanVerbose(plan *engine.Plan, color bool) string {
	var blocks []string
	for _, c := range plan.Changes {
		switch c.Type {
		case engine.ChangeCreate:
			blocks = append(blocks, formatCreate(c, color))
		case engine.ChangeUpdate:
			blocks = append(blocks, formatVerboseUpdate(c, color))
		case engine.ChangeDelete:
			blocks = append(blocks, formatVerboseDelete(c, color))
		}
	}
	if len(blocks) == 0 {
		return "No changes."
	}
	return strings.Join(blocks, "\n\n") + "\n\n" + bold(plan.Summary(), color) + "\n"
}

func formatVerboseUpdate(c engine.ResourceChange, color bool) string {
	var b strings.Builder
	header := fmt.Sprintf("~ %s (update)", c.ID)
	b.WriteString(yellow(header, color))

	// Diffs.
	b.WriteString("\n  diffs:")
	for _, d := range c.Diff.Diffs {
		b.WriteString("\n    ")
		b.WriteString(d.Path)
		b.WriteString(": ")
		b.WriteString(formatDiffValue(d, color))
	}

	// Full desired body.
	if c.Desired != nil && c.Desired.Body != nil {
		b.WriteString("\n  desired:")
		writeBody(&b, c.Desired.Body, color, ansiGreen)
	}

	// Full live body.
	if c.Live != nil && c.Live.Body != nil {
		b.WriteString("\n  live:")
		writeBody(&b, c.Live.Body, color, ansiRed)
	}

	return b.String()
}

func formatVerboseDelete(c engine.ResourceChange, color bool) string {
	var b strings.Builder
	header := fmt.Sprintf("- %s (delete)", c.ID)
	b.WriteString(red(header, color))

	if c.Live != nil && c.Live.Body != nil {
		writeBody(&b, c.Live.Body, color, ansiRed)
	}

	return b.String()
}

func writeBody(b *strings.Builder, body *provider.OrderedMap, color bool, ansiCode string) {
	for _, key := range body.Keys() {
		v, _ := body.Get(key)
		line := fmt.Sprintf("\n    %s: %s", key, v.String())
		if color && ansiCode != "" {
			b.WriteString(ansiCode + line + ansiReset)
		} else {
			b.WriteString(line)
		}
	}
}
