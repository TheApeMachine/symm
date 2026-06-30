package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theapemachine/symm/playbook/optimizer"
)

func init() {
	rootCmd.AddCommand(newOptimizePlaybookCommand())
}

func newOptimizePlaybookCommand() *cobra.Command {
	var auditPath string
	var symbolList string
	var replayPath string
	var iterations int
	var depth int
	var exploration float64
	var causalAlpha float64
	var initialCash float64
	var feeRate float64
	var makerFeeRate float64
	var maxPositions int
	var lookback time.Duration
	var interval int
	var maxPairs int
	var writeTree bool
	var treePath string
	var backupPath string

	command := &cobra.Command{
		Use:   "optimize-playbook",
		Short: "optimize tree.yml by replaying Kraken history through CausalMCTS",
		RunE: func(cmd *cobra.Command, args []string) error {
			progressf := func(format string, args ...any) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[optimize] "+format+"\n", args...)
			}

			progressf("reading playbook tree: %s", treePath)
			treeBytes, err := os.ReadFile(treePath)
			if err != nil {
				return fmt.Errorf("read tree: %w", err)
			}

			symbols := parseSymbols(symbolList)
			if len(symbols) == 0 {
				symbols = symbolsFromAudit(auditPath)
			}
			if len(symbols) == 0 {
				symbols = []string{"ADA/USD", "BTC/USD", "DOGE/USD", "ETH/USD", "SOL/USD", "XRP/USD"}
			}
			progressf("symbol universe: %d pairs", len(symbols))

			progressf("loading replay frames: replay=%s lookback=%s interval=%dm", replayPath, lookback, interval)
			started := time.Now()
			frames, source, err := loadReplayFrames(cmd.Context(), replayPath, symbols, optimizer.HistoricalOptions{
				Lookback: lookback,
				Interval: interval,
				MaxPairs: maxPairs,
			})
			if err != nil {
				return err
			}
			progressf(
				"loaded replay frames: source=%s frames=%d elapsed=%s",
				source,
				len(frames),
				time.Since(started).Round(time.Millisecond),
			)

			progressf("starting CausalMCTS replay optimization")
			report, nextTree, err := optimizer.Optimize(treeBytes, frames, optimizer.Options{
				Iterations:   iterations,
				MaxDepth:     depth,
				Exploration:  exploration,
				CausalAlpha:  causalAlpha,
				InitialCash:  initialCash,
				FeeRate:      feeRate,
				MakerFeeRate: makerFeeRate,
				MaxPositions: maxPositions,
				Progressf:    progressf,
			})
			if err != nil {
				return err
			}
			progressf(
				"optimizer selected: mutations=%s wallet=%.2f reward=%.4f",
				strings.Join(report.Best.Mutations, " + "),
				report.Best.Wallet,
				report.Best.Reward,
			)

			output := struct {
				optimizer.Report
				TreeWritten bool   `json:"tree_written,omitempty"`
				Backup      string `json:"backup,omitempty"`
			}{Report: report}

			if writeTree {
				progressf("writing optimized tree if changed: %s", treePath)
				backup, written, err := writeOptimizedTree(treePath, backupPath, treeBytes, nextTree)
				if err != nil {
					return err
				}
				output.TreeWritten = written
				output.Backup = backup
				if written {
					progressf("tree written: backup=%s", backup)
				} else {
					progressf("tree unchanged: no write needed")
				}
			}

			encoded, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
			return nil
		},
	}

	command.Flags().StringVar(&auditPath, "input", "runs/audit.jsonl", "optional audit JSONL used only to discover symbols when --symbols is empty")
	command.Flags().StringVar(&auditPath, "audit", "runs/audit.jsonl", "optional audit JSONL used only to discover symbols when --symbols is empty")
	command.Flags().StringVar(&replayPath, "replay", "runs/replay.jsonl", "raw replay JSONL; if present, this is used instead of Kraken public history")
	command.Flags().StringVar(&symbolList, "symbols", "", "comma-separated symbol universe, e.g. BTC/USD,ETH/USD")
	command.Flags().IntVar(&iterations, "iterations", 0, "MCTS iterations; 0 uses the optimizer default")
	command.Flags().IntVar(&depth, "depth", 0, "maximum playbook mutation depth per rollout; 0 uses the optimizer default")
	command.Flags().Float64Var(&exploration, "exploration", 0, "MCTS exploration constant; 0 uses the optimizer default")
	command.Flags().Float64Var(&causalAlpha, "causal-alpha", 0, "causal adjustment weight; 0 uses the optimizer default")
	command.Flags().Float64Var(&initialCash, "initial-cash", 200, "starting replay wallet cash")
	command.Flags().Float64Var(&feeRate, "fee-rate", 0.004, "taker per-fill fee rate used by the replay ledger")
	command.Flags().Float64Var(&makerFeeRate, "maker-fee-rate", 0.0025, "maker per-fill fee rate used by limit-style replay fills")
	command.Flags().IntVar(&maxPositions, "max-positions", 3, "maximum concurrent replay positions")
	command.Flags().DurationVar(&lookback, "lookback", 6*time.Hour, "historical replay lookback")
	command.Flags().IntVar(&interval, "interval", 1, "Kraken OHLC interval in minutes")
	command.Flags().IntVar(&maxPairs, "max-pairs", 16, "maximum symbol pairs to fetch when deriving symbols from audit")
	command.Flags().BoolVar(&writeTree, "write-tree", false, "backup and rewrite tree.yml with the best replayed playbook")
	command.Flags().StringVar(&treePath, "tree", "logic/rules/tree.yml", "playbook tree.yml path")
	command.Flags().StringVar(&backupPath, "backup", "", "tree backup path; default is tree.bak next to --tree")

	return command
}

func loadReplayFrames(
	ctx context.Context,
	replayPath string,
	symbols []string,
	options optimizer.HistoricalOptions,
) ([]optimizer.ReplayFrame, string, error) {
	if strings.TrimSpace(replayPath) != "" {
		file, err := os.Open(replayPath)
		if err == nil {
			defer file.Close()
			frames, readErr := optimizer.ReadReplayJSONL(file)
			if readErr != nil {
				return nil, "", readErr
			}
			if len(frames) > 0 {
				optimizer.SortReplayFrames(frames)
				return frames, replayPath, nil
			}
		}
	}

	frames, err := optimizer.BuildHistoricalReplay(ctx, symbols, options)
	if err != nil {
		return nil, "", err
	}

	return frames, "Kraken public history", nil
}

func parseSymbols(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	return uniqueSymbols(strings.Split(input, ","))
}

func symbolsFromAudit(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	symbols := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			continue
		}
		if symbol, ok := row["symbol"].(string); ok {
			symbols = append(symbols, symbol)
		}
		if decisions, ok := row["decisions"].([]any); ok {
			for _, item := range decisions {
				decision, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if symbol, ok := decision["symbol"].(string); ok {
					symbols = append(symbols, symbol)
				}
			}
		}
	}

	return uniqueSymbols(symbols)
}

func uniqueSymbols(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	symbols := make([]string, 0, len(values))
	for _, value := range values {
		symbol := strings.ToUpper(strings.TrimSpace(value))
		if symbol == "" || !strings.Contains(symbol, "/") {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	return symbols
}

func writeOptimizedTree(treePath string, backupPath string, current []byte, next []byte) (string, bool, error) {
	if len(next) == 0 {
		return "", false, fmt.Errorf("optimizer produced an empty tree")
	}
	if bytes.Equal(current, next) {
		return "", false, nil
	}
	if backupPath == "" {
		backupPath = defaultTreeBackupPath(treePath)
	}
	if backupPath == treePath {
		return "", false, fmt.Errorf("backup path must differ from tree path: %s", treePath)
	}

	mode := os.FileMode(0o644)
	if stat, statErr := os.Stat(treePath); statErr == nil {
		mode = stat.Mode()
	}

	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return "", false, fmt.Errorf("create backup dir: %w", err)
	}
	if err := os.WriteFile(backupPath, current, mode); err != nil {
		return "", false, fmt.Errorf("write tree backup: %w", err)
	}
	if err := os.WriteFile(treePath, next, mode); err != nil {
		return "", false, fmt.Errorf("write optimized tree: %w", err)
	}

	return backupPath, true, nil
}

func defaultTreeBackupPath(treePath string) string {
	ext := filepath.Ext(treePath)
	if ext == "" {
		return treePath + ".bak"
	}

	return strings.TrimSuffix(treePath, ext) + ".bak"
}
