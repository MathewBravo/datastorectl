package main

import (
	"fmt"
	"os"

	"github.com/MathewBravo/datastorectl/config"
	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/output"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Parse and type-check DCL offline — no network calls",
	Long: `Validate parses the DCL file or directory, checks for syntax errors,
converts resource blocks, and validates context references. No cluster
connection is made — this is a purely offline check.`,
	Args: cobra.ExactArgs(1),
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	path := args[0]
	color := colorEnabled(cmd)
	format := outputFormat(cmd)

	// 1. Load and parse DCL.
	file, err := loadDCL(path)
	if err != nil {
		if format == "json" {
			// Parse errors are in the error string; wrap as diagnostic JSON.
			fmt.Println(`{"valid": false, "error": ` + jsonString(err.Error()) + `}`)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return errExit{code: 1}
	}

	// 2. Split into context and resource blocks.
	contextBlocks, resourceBlocks := config.SplitFile(file)

	// 3. Validate context blocks.
	contexts, err := config.ParseContexts(contextBlocks)
	if err != nil {
		return exitWithError(cmd, err, format, color)
	}

	// 4. Convert resource blocks.
	resourceSet, err := engine.ConvertBlocks(resourceBlocks)
	if err != nil {
		return exitWithError(cmd, err, format, color)
	}

	// 5. Validate context references on resources (if contexts exist).
	if len(contexts) > 0 {
		if _, err := config.ResolveResourceContexts(resourceSet.Resources, contexts); err != nil {
			return exitWithError(cmd, err, format, color)
		}
	}

	// All checks pass.
	if format == "json" {
		fmt.Println(`{"valid": true}`)
	} else {
		fmt.Println("Valid.")
	}
	return nil
}

func exitWithError(cmd *cobra.Command, err error, format string, color bool) error {
	if format == "json" {
		fmt.Println(`{"valid": false, "error": ` + jsonString(err.Error()) + `}`)
	} else {
		// Use diagnostic formatting for richer errors when available.
		fmt.Fprintln(os.Stderr, output.FormatDiagnostics(errToDiag(err), color))
	}
	return errExit{code: 1}
}

// errToDiag wraps a plain error as a single error-severity diagnostic.
func errToDiag(err error) dcl.Diagnostics {
	return dcl.Diagnostics{{
		Severity: dcl.SeverityError,
		Message:  err.Error(),
	}}
}

// jsonString returns a JSON-encoded string value.
func jsonString(s string) string {
	// Simple escaping for JSON string values.
	out := `"`
	for _, r := range s {
		switch r {
		case '"':
			out += `\"`
		case '\\':
			out += `\\`
		case '\n':
			out += `\n`
		case '\r':
			out += `\r`
		case '\t':
			out += `\t`
		default:
			out += string(r)
		}
	}
	return out + `"`
}
