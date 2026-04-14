package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MathewBravo/datastorectl/config"
	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/output"
	"github.com/MathewBravo/datastorectl/provider"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [path]",
	Short: "Connect to the cluster, diff against declared state, show what would change",
	Long: `Plan reads the DCL file or directory, connects to the cluster, discovers
live state, and computes a diff against the declared configuration.
Changes are displayed using Terraform conventions: + create, ~ update, - delete.

Exit codes: 0 = no changes, 1 = error, 2 = drift detected (changes pending).`,
	Args: cobra.ExactArgs(1),
	RunE: runPlan,
}

func init() {
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	path := args[0]
	color := colorEnabled(cmd)
	format := outputFormat(cmd)
	verbose := isVerbose(cmd)
	ctx := context.Background()

	// 1. Load DCL.
	file, err := loadDCL(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return errExit{code: 1}
	}

	// 2. Load external config file and merge contexts.
	cfgPath := configPath(cmd)
	fileContexts, err := config.LoadConfigFile(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return errExit{code: 1}
	}

	// Merge external contexts into the file so the engine sees them.
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
		// Peek at resources for multi-context detection.
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

	// 4. Run the engine plan.
	eng := createEngine()
	plan, _, err := eng.Plan(ctx, file, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return errExit{code: 1}
	}

	// 5. Format output.
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

	// 6. Exit code: 2 for drift, 0 for clean.
	if plan.HasChanges() {
		return errExit{code: 2}
	}
	return nil
}

// convertAndResolveForDetection does a lightweight convert of resource blocks
// for multi-context detection. Errors are swallowed — detection is best-effort.
func convertAndResolveForDetection(resourceBlocks []dcl.Block, contexts []config.Context) ([]provider.Resource, error) {
	rs, err := engine.ConvertBlocks(resourceBlocks)
	if err != nil {
		return nil, err
	}
	return rs.Resources, nil
}
