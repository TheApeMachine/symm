package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theapemachine/symm/playbook/optimizer"
)

func init() {
	rootCmd.AddCommand(newOptimizePlaybookCommand())
}

func newOptimizePlaybookCommand() *cobra.Command {
	var input string
	var iterations int
	var holdout float64
	var exploration float64
	var causalAlpha float64
	var minRows int
	var linearFit bool
	var writeTree bool
	var treePath string
	var backupPath string

	command := &cobra.Command{
		Use:   "optimize-playbook",
		Short: "rank playbook action families with causal MCTS",
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(input)
			if err != nil {
				return fmt.Errorf("open optimizer input: %w", err)
			}
			defer file.Close()

			samples, err := optimizer.ReadJSONL(file)
			if err != nil {
				return err
			}

			report, err := optimizer.Optimize(samples, optimizer.Options{
				Iterations:      iterations,
				HoldoutFraction: holdout,
				Exploration:     exploration,
				CausalAlpha:     causalAlpha,
				MinRows:         minRows,
				LinearFit:       linearFit,
			})
			if err != nil {
				return err
			}

			output := struct {
				optimizer.Report
				Tree   *optimizer.TreeRewrite `json:"tree,omitempty"`
				Backup string                 `json:"backup,omitempty"`
			}{Report: report}

			if writeTree {
				rewrite, backup, err := rewriteTreeFile(treePath, backupPath, report)
				if err != nil {
					return err
				}
				output.Tree = &rewrite
				output.Backup = backup
			}

			encoded, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
			return nil
		},
	}

	command.Flags().StringVar(&input, "input", "runs/audit.jsonl", "JSONL rows with explicit reward/edge, friction, and fill data")
	command.Flags().IntVar(&iterations, "iterations", 0, "MCTS iterations; 0 derives from sample and action counts")
	command.Flags().Float64Var(&holdout, "holdout", 0.25, "trailing sample fraction reserved for out-of-sample scoring")
	command.Flags().Float64Var(&exploration, "exploration", 1, "MCTS exploration constant")
	command.Flags().Float64Var(&causalAlpha, "causal-alpha", 1, "causal adjustment weight")
	command.Flags().IntVar(&minRows, "min-rows", 0, "minimum rows before causal adjustment; 0 derives from action count")
	command.Flags().BoolVar(&linearFit, "linear-fit", false, "use linear counterfactual fit in the causal adapter")
	command.Flags().BoolVar(&writeTree, "write-tree", false, "rewrite tree.yml from the optimizer report after backing it up")
	command.Flags().StringVar(&treePath, "tree", "logic/rules/tree.yml", "playbook tree.yml path to rewrite with --write-tree")
	command.Flags().StringVar(&backupPath, "backup", "", "tree backup path; default is tree.bak next to --tree")

	return command
}

func rewriteTreeFile(
	treePath string,
	backupPath string,
	report optimizer.Report,
) (optimizer.TreeRewrite, string, error) {
	current, err := os.ReadFile(treePath)
	if err != nil {
		return optimizer.TreeRewrite{}, "", fmt.Errorf("read tree: %w", err)
	}

	next, rewrite, err := optimizer.RewriteTreeYAML(current, report)
	if err != nil {
		return optimizer.TreeRewrite{}, "", err
	}

	if backupPath == "" {
		backupPath = defaultTreeBackupPath(treePath)
	}
	if backupPath == treePath {
		return optimizer.TreeRewrite{}, "", fmt.Errorf("backup path must differ from tree path: %s", treePath)
	}

	mode := os.FileMode(0o644)
	if stat, statErr := os.Stat(treePath); statErr == nil {
		mode = stat.Mode()
	}

	if err := os.WriteFile(backupPath, current, mode); err != nil {
		return optimizer.TreeRewrite{}, "", fmt.Errorf("write tree backup: %w", err)
	}
	if err := os.WriteFile(treePath, next, mode); err != nil {
		return optimizer.TreeRewrite{}, "", fmt.Errorf("write optimized tree: %w", err)
	}

	return rewrite, backupPath, nil
}

func defaultTreeBackupPath(treePath string) string {
	ext := filepath.Ext(treePath)
	if ext == "" {
		return treePath + ".bak"
	}
	return strings.TrimSuffix(treePath, ext) + ".bak"
}
