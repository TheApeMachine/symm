package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/replay"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/toxicity"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/trader/economics"
	"github.com/theapemachine/symm/view"
	"github.com/theapemachine/symm/wallet"
)

type engineResult struct {
	Wallet      *wallet.Wallet
	Regret      economics.RegretSummary
	Performance economics.PerformanceSummary
}

func bootEngine(ctx context.Context) (*engineResult, error) {
	market.ResetBookHealth()
	broker.ResetStressMachine()

	if err := configurePerspectives(config.PerspectiveLoadPath()); err != nil {
		return nil, err
	}

	if path := strings.TrimSpace(config.System.RecordFile); path != "" {
		if _, err := replay.OpenRecorder(path); err != nil {
			return nil, err
		}
	}

	workerCount, err := engineWorkerCount()

	if err != nil {
		return nil, err
	}

	pool := qpool.NewQ(ctx, 1, workerCount, qpool.NewConfig())
	restoreLogging := qpool.SuppressLogging()
	defer restoreLogging()

	if replayPath := strings.TrimSpace(config.System.ReplayFile); replayPath != "" {
		applyReplayMeta(replayPath)
	} else if universe := market.DiscoverSymbols(ctx, config.System.QuoteCurrency); len(universe) > 0 {
		universe = market.LimitSymbols(universe, config.System.MaxScanSymbols)
		config.System.Symbols = universe
		config.SyncRuntime()

		if recorder := replay.ActiveRecorder(); recorder != nil {
			_ = replay.WriteMeta("symbols", map[string]any{
				"quote_currency": config.System.QuoteCurrency,
				"symbols":        universe,
			})
		}
	}

	market.ConfigureCatalogFees(
		config.System.Fee30DVolume,
		config.System.TakerFeePct,
		config.System.MakerFeePct,
	)

	if err := market.ConfigureLevel3(config.System.KrakenAPIKey, config.System.KrakenAPISecret); err != nil {
		return nil, fmt.Errorf("level3: %w", err)
	}

	market.BootPairCatalog(
		ctx,
		config.System.Fee30DVolume,
		config.System.TakerFeePct,
		config.System.MakerFeePct,
	)

	tracker := focus.NewSet()
	tradingWallet := newTradingWallet()

	booter, err := NewBooter(ctx, pool)

	if err != nil {
		return nil, err
	}

	runCtx := booter.Context()
	tradingCrypto, err := trader.NewCrypto(runCtx, pool, tradingWallet, tracker, config.Runtime)

	if err != nil {
		return nil, err
	}

	systems := []System{
		pumpdump.NewSignal(runCtx, pool),
		correlation.NewSignal(runCtx, pool),
		depthflow.NewSignal(runCtx, pool),
		hawkes.NewSignal(runCtx, pool),
		leadlag.NewSignal(runCtx, pool),
		liquidity.NewSignal(runCtx, pool),
		sentiment.NewSignal(runCtx, pool),
		fluid.NewSignal(runCtx, pool, tracker),
		causal.NewSignal(runCtx, pool),
		cvd.NewSignal(runCtx, pool),
		toxicity.NewToxicity(runCtx, pool),
		exhaust.NewSignal(runCtx, pool),
		tradingCrypto,
	}

	if !config.System.Headless {
		systems = append(systems,
			view.NewOHLC(runCtx, pool, tracker),
			view.NewGauges(runCtx, pool),
		)
	}

	if err := booter.AddSystems(systems...); err != nil {
		return nil, err
	}

	if done := market.ReplayDone(); done != nil {
		go func() {
			<-done
			booter.Cancel()
		}()
	}

	if err := booter.Boot(); err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}

	if recorder := replay.ActiveRecorder(); recorder != nil {
		_ = recorder.Close()
	}

	tradingCrypto.FlushGateRejectRegret()

	if config.System.Headless {
		tradingCrypto.FlushOpenPositionPerformance()
	}

	return &engineResult{
		Wallet:      tradingWallet,
		Regret:      tradingCrypto.GateRegretSummary(),
		Performance: tradingCrypto.PerformanceSummary(),
	}, nil
}

func applyReplayMeta(replayPath string) {
	hub, err := replay.Open(replayPath)

	if err != nil {
		errnie.Error(err)

		return
	}

	meta, ok := hub.Meta("symbols")

	if !ok {
		return
	}

	var payload struct {
		Symbols []string `json:"symbols"`
	}

	if err := json.Unmarshal(meta, &payload); err != nil {
		errnie.Error(err)

		return
	}

	if len(payload.Symbols) > 0 {
		config.System.Symbols = payload.Symbols
	}
}

func walletScore(tradingWallet *wallet.Wallet) float64 {
	if tradingWallet == nil {
		return 0
	}

	snapshot := tradingWallet.Snapshot()
	marks := make(map[string]float64, len(snapshot.Marks))

	for base, mark := range snapshot.Marks {
		marks[base] = mark
	}

	return tradingWallet.MarkEquity(marks)
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run one replay-backed evaluation and print wallet score JSON",
	Run: func(cmd *cobra.Command, args []string) {
		config.System.Headless = true

		result, err := bootEngine(cmd.Context())

		if err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		score := walletScore(result.Wallet)
		start := config.System.WalletEUR
		scoreEUR := score - start
		fitnessEUR := TuneFitness(scoreEUR, result.Regret.MissedForwardEUR, result.Performance)
		bookHealth := market.BookIntegritySummary()

		payload, err := json.Marshal(map[string]any{
			"score_eur":              scoreEUR,
			"fitness_eur":            fitnessEUR,
			"equity_eur":             score,
			"wallet_eur":             start,
			"balance_eur":            result.Wallet.BalanceCopy(),
			"open_bases":             len(result.Wallet.Snapshot().Inventory),
			"gate_reject_regret":     result.Regret,
			"trade_performance":      result.Performance,
			"book_diverged_symbols":  bookHealth.DivergedSymbols,
			"book_divergence_events": bookHealth.DivergenceEvents,
			"book_diverged":          bookHealth.Diverged,
		})

		if err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		if bookHealth.DivergedSymbols > 0 {
			sample := bookHealth.Diverged

			if len(sample) > 5 {
				sample = sample[:5]
			}

			fmt.Fprintf(
				os.Stderr,
				"symm eval: %d symbols lost book checksum sync (e.g. %s) — fluid/depthflow blind for those symbols\n",
				bookHealth.DivergedSymbols,
				strings.Join(sample, ", "),
			)
		}

		fmt.Println(string(payload))
	},
}

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Search tunable config against a replay fixture to maximize fitness",
	Run: func(cmd *cobra.Command, args []string) {
		result, err := runTune(cmd.Context(), cmd)

		if err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		fmt.Println(string(result.payload))
	},
}

func init() {
	rootCmd.AddCommand(evalCmd)
	tuneCmd.Flags().String("replay", defaultReplayFile, "Replay JSONL fixture path")
	tuneCmd.Flags().StringArray("holdout", nil, "Holdout replay JSONL paths; best config is chosen by minimum holdout fitness")
	tuneCmd.Flags().Bool("auto-holdout", true, "Reserve the last 20% of --replay as holdout when --holdout is unset")
	tuneCmd.Flags().Int("walk-forward-folds", replay.DefaultWalkForwardFolds(), "Walk-forward holdout folds (1 = single tail holdout)")
	tuneCmd.Flags().Bool("replay-perturb", true, "Apply quantity/timestamp jitter on train replay evals")
	tuneCmd.Flags().Bool("stress-holdout", true, "Run holdout evals with execution stress enabled")
	tuneCmd.Flags().Float64("max-train-holdout-gap", 0, "Reject candidates when train minus min holdout exceeds this EUR (0 = 3% of wallet; -1 = disable)")
	tuneCmd.Flags().Int("iterations", 0, "Max search trials (0 = run until Ctrl+C; saves best on interrupt)")
	tuneCmd.Flags().Int("workers", runtime.NumCPU(), "Concurrent eval workers")
	tuneCmd.Flags().String("output", config.DefaultTunedPath(), "Path to write best tunables")
	tuneCmd.Flags().String("perspectives-output", config.DefaultPerspectivePath(), "Path to write best perspective tree YAML")
	tuneCmd.Flags().Bool("quiet", false, "Suppress human-readable progress on stderr (JSON on stdout only)")
	rootCmd.AddCommand(tuneCmd)
}
