package cmd

import (
	"fmt"
	"os"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	ptypes "github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/optimizer/replay"
)

/*
replayCmd is the manual workbench: score ONE hand-written playbook against a
recorded capture and print the per-setup scoreboard — no search, no waiting.
Edit the YAML, run `symm replay`, read what each branch would have done. This is
the fast loop for authoring setups; the optimizer only comes back later to
calibrate numbers on structures validated here.
*/
var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Score a playbook against recorded run data (per-setup scoreboard)",
	RunE: func(cmd *cobra.Command, args []string) error {
		playbookFile, _ := cmd.Flags().GetString("playbook")

		if strings.TrimSpace(playbookFile) == "" {
			playbookFile = tunePerspectivesPath()
		}

		capturePath, _ := cmd.Flags().GetString("capture")

		if strings.TrimSpace(capturePath) == "" {
			var err error
			capturePath, err = tuneMeasurementPath()

			if err != nil {
				return err
			}
		}

		maxMeasurements, _ := cmd.Flags().GetInt("max-measurements")
		tradesTail, _ := cmd.Flags().GetInt("trades")
		workers, _ := cmd.Flags().GetInt("workers")

		if workers <= 0 {
			workers = goruntime.NumCPU()
		}

		raw, err := os.ReadFile(playbookFile)

		if err != nil {
			return fmt.Errorf("replay: playbook %q: %w", playbookFile, err)
		}

		thoughts, err := reasoning.ParseThoughts(raw)

		if err != nil {
			return fmt.Errorf("replay: parse %q: %w", playbookFile, err)
		}

		if len(thoughts) == 0 {
			return fmt.Errorf("replay: %q contains no branches", playbookFile)
		}

		rows, skipped, err := loadTuneMeasurements(capturePath, maxMeasurements)

		if err != nil {
			return err
		}

		if skipped > 0 {
			fmt.Fprintf(os.Stderr, "replay: skipped %d malformed lines in %s\n", skipped, capturePath)
		}

		sort.SliceStable(rows, func(left, right int) bool {
			return rows[left].At.Before(rows[right].At)
		})

		costs := replay.DefaultReplayCosts()
		costs.CollectTrades = true

		if viper.GetBool("optimizer.tune.load_instrument_rules") {
			rules, loaded, rulesErr := broker.LoadInstrumentRulesFromKraken(cmd.Context())

			if rulesErr != nil {
				fmt.Fprintf(os.Stderr, "replay: instrument rules unavailable (%v) — exchange minimums NOT enforced\n", rulesErr)
			} else {
				costs.InstrumentRules = rules
				fmt.Fprintf(os.Stderr, "replay: %d instrument rules loaded\n", loaded)
			}
		}

		tape, err := replay.PrecompileTapeWorkers(rows, workers)

		if err != nil {
			return err
		}

		result := replay.NewThoughtSimulation(cmd.Context(), thoughts, tape, costs).Result()
		printReplayReport(playbookFile, rows, result, tradesTail)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(replayCmd)

	replayCmd.Flags().String("playbook", "", "playbook YAML to score (default: the live playbook path)")
	replayCmd.Flags().String("capture", "", "measurement capture to replay (default: trading.record.file)")
	replayCmd.Flags().Int("max-measurements", 0, "limit rows loaded from the capture; 0 loads all")
	replayCmd.Flags().Int("trades", 15, "how many of the most recent round trips to print")
	replayCmd.Flags().Int("workers", 0, "tape precompile workers; 0 uses all CPUs")
}

func printReplayReport(
	playbookFile string,
	rows []ptypes.Measurement,
	result replay.ReplayResult,
	tradesTail int,
) {
	span := ""

	if len(rows) > 0 {
		span = fmt.Sprintf(
			" · %s → %s",
			rows[0].At.Format("2006-01-02 15:04:05"),
			rows[len(rows)-1].At.Format("15:04:05"),
		)
	}

	fmt.Printf("replay: %s · %d rows%s · capital €%.2f\n",
		playbookFile, len(rows), span, result.StartingCapital)

	exposurePct := 0.0

	if result.TotalTicks > 0 {
		exposurePct = 100 * float64(result.ExposureTicks) / float64(result.TotalTicks)
	}

	fmt.Printf(
		"summary: realized €%+.4f (%+.3f%%) · %d round trips · exposure %.1f%% · blocked[fund=%d preflight=%d exit=%d]\n\n",
		result.RealizedEUR,
		100*result.Score,
		result.ClosedTrades,
		exposurePct,
		result.FundBlocked,
		result.PreflightBlocked,
		result.ExitBlocked,
	)

	if len(result.PerStrategy) > 0 {
		names := make([]string, 0, len(result.PerStrategy))

		for name := range result.PerStrategy {
			names = append(names, name)
		}

		sort.Slice(names, func(left, right int) bool {
			return result.PerStrategy[names[left]].RealizedEUR > result.PerStrategy[names[right]].RealizedEUR
		})

		fmt.Printf("%-28s %7s %6s %12s %10s\n", "setup", "trades", "win%", "realized", "avg hold")

		for _, name := range names {
			setup := result.PerStrategy[name]
			winPct := 0.0

			if setup.Trades > 0 {
				winPct = 100 * float64(setup.Wins) / float64(setup.Trades)
			}

			fmt.Printf("%-28s %7d %5.0f%% %11s€ %10s\n",
				name,
				setup.Trades,
				winPct,
				fmt.Sprintf("%+.4f", setup.RealizedEUR),
				formatHold(setup.AvgHoldSeconds),
			)
		}

		fmt.Println()
	}

	if tradesTail <= 0 || len(result.Trades) == 0 {
		if result.ClosedTrades == 0 {
			fmt.Println("no round trips closed — check the blocked counters above and the branch hold rates on the dashboard")
		}

		return
	}

	trades := result.Trades

	if len(trades) > tradesTail {
		trades = trades[len(trades)-tradesTail:]
	}

	fmt.Printf("last %d round trips:\n", len(trades))

	for _, trade := range trades {
		fmt.Printf("%s  %-22s %-12s %12.6g → %-12.6g %9s€  hold %s\n",
			trade.ExitAt.Format("15:04:05"),
			trade.Strategy,
			trade.Symbol,
			trade.EntryPrice,
			trade.ExitPrice,
			fmt.Sprintf("%+.4f", trade.RealizedEUR),
			formatHold(trade.ExitAt.Sub(trade.EntryAt).Seconds()),
		)
	}
}

func formatHold(seconds float64) string {
	if seconds <= 0 {
		return "-"
	}

	duration := time.Duration(seconds * float64(time.Second))

	if duration < time.Minute {
		return fmt.Sprintf("%.0fs", duration.Seconds())
	}

	if duration < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(duration.Minutes()), int(duration.Seconds())%60)
	}

	return fmt.Sprintf("%dh%02dm", int(duration.Hours()), int(duration.Minutes())%60)
}
