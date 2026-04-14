package main

import (
	"fmt"
	"os"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/output"
	"github.com/spf13/cobra"
)

// loadDCL loads a DCL file or directory. Returns the parsed file or an error
// wrapping any parse diagnostics.
func loadDCL(path string) (*dcl.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %s", path, err)
	}

	var file *dcl.File
	var diags dcl.Diagnostics

	if info.IsDir() {
		file, diags = dcl.LoadDirectory(path)
	} else {
		file, diags = dcl.LoadFile(path)
	}

	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	return file, nil
}

// createEngine returns an Engine configured with the default EnvSecretResolver.
func createEngine() *engine.Engine {
	return &engine.Engine{SecretResolver: engine.EnvSecretResolver{}}
}

// colorEnabled returns true when colored output should be used.
// Respects --no-color flag and TTY detection.
func colorEnabled(cmd *cobra.Command) bool {
	noColor, _ := cmd.Flags().GetBool("no-color")
	if noColor {
		return false
	}
	return output.ShouldColor(os.Stdout)
}

// outputFormat returns the --output flag value ("text" or "json").
func outputFormat(cmd *cobra.Command) string {
	format, _ := cmd.Flags().GetString("output")
	return format
}

// isVerbose returns whether the --verbose flag is set.
func isVerbose(cmd *cobra.Command) bool {
	verbose, _ := cmd.Flags().GetBool("verbose")
	return verbose
}

// configPath returns the --config flag value.
func configPath(cmd *cobra.Command) string {
	path, _ := cmd.Flags().GetString("config")
	return path
}

// contextFlag returns the --context flag value (empty string if not set).
func contextFlag(cmd *cobra.Command) string {
	ctx, _ := cmd.Flags().GetString("context")
	return ctx
}
