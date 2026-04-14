package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MathewBravo/datastorectl/config"
	"github.com/MathewBravo/datastorectl/output"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply [path]",
	Short: "Execute planned changes against the cluster",
	Long: `Apply reads the DCL file or directory, connects to the cluster, computes
a plan, and executes all changes. Use --dry-run to validate the full
pipeline without making changes.

Exit codes: 0 = all changes succeeded, 1 = one or more changes failed.`,
	Args: cobra.ExactArgs(1),
	RunE: runApply,
}

func init() {
	applyCmd.Flags().Bool("dry-run", false, "validate the full pipeline without applying changes")
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	path := args[0]
	color := colorEnabled(cmd)
	format := outputFormat(cmd)
	verbose := isVerbose(cmd)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	ctx := context.Background()

	// 1. Load DCL.
	file, err := loadDCL(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return errExit{code: 1}
	}

	// 2. Load external config file.
	cfgPath := configPath(cmd)
	fileContexts, err := config.LoadConfigFile(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return errExit{code: 1}
	}

	// Merge external contexts.
	if len(fileContexts) > 0 {
		inlineBlocks, _ := config.SplitFile(file)
		inlineContexts, _ := config.ParseContexts(inlineBlocks)
		if _, err := config.MergeContexts(inlineContexts, fileContexts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return errExit{code: 1}
		}
	}

	// 3. Multi-context check.
	ctxFlag := contextFlag(cmd)
	if ctxFlag == "" {
		contextBlocks, resourceBlocks := config.SplitFile(file)
		if len(contextBlocks) > 0 {
			contexts, err := config.ParseContexts(contextBlocks)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return errExit{code: 1}
			}
			rs, err := convertAndResolveForDetection(resourceBlocks, contexts)
			if err == nil {
				names, multiple := config.DetectMultipleContexts(rs)
				if multiple {
					fmt.Fprintf(os.Stderr, "Error: resources target multiple contexts (%s).\nUse --context to select one.\n", strings.Join(names, ", "))
					return errExit{code: 1}
				}
			}
		}
	}

	eng := createEngine()

	// 4. Dry run path.
	if dryRun {
		plan, err := eng.DryRun(ctx, file, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return errExit{code: 1}
		}

		switch format {
		case "json":
			data, err := output.FormatPlanJSON(plan)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return errExit{code: 1}
			}
			fmt.Println(string(data))
		default:
			if verbose {
				fmt.Print(output.FormatPlanVerbose(plan, color))
			} else {
				fmt.Print(output.FormatPlan(plan, color))
			}
		}

		fmt.Println("\nDry run complete. No changes applied.")

		if plan.HasChanges() {
			return errExit{code: 2}
		}
		return nil
	}

	// 5. Full apply path.
	result, err := eng.Apply(ctx, file, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return errExit{code: 1}
	}

	switch format {
	case "json":
		data, err := output.FormatApplyResultJSON(result)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return errExit{code: 1}
		}
		fmt.Println(string(data))
	default:
		fmt.Print(output.FormatApplyResult(result, color))
	}

	if result.HasErrors() {
		return errExit{code: 1}
	}
	return nil
}
