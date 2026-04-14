package main

import (
	"fmt"
	"os"

	"github.com/MathewBravo/datastorectl/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "datastorectl [command] [path]",
	Short: "Declarative configuration for running datastores",
	Long: `datastorectl manages post-provisioning datastore configuration declaratively,
the same way Terraform manages infrastructure.

Write config in DCL, point it at a live cluster, and run three commands:
  validate  — parse and type-check DCL offline, no network calls
  plan      — connect to the cluster, diff against declared state, show what would change
  apply     — execute the changes`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().String("context", "", "select a named context when resources target multiple clusters")
	rootCmd.PersistentFlags().StringP("output", "o", "text", "output format: text or json")
	rootCmd.PersistentFlags().Bool("verbose", false, "show full before/after in plan output")
	rootCmd.PersistentFlags().Bool("no-color", false, "disable colored output")
	rootCmd.PersistentFlags().String("config", config.DefaultConfigPath(), "path to config file")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
