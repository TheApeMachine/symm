package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
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
	"github.com/theapemachine/symm/view"
	"github.com/theapemachine/symm/wallet"
)

type engineResult struct {
	Wallet *wallet.Wallet
}

func bootEngine(ctx context.Context) (*engineResult, error) {
	if path := strings.TrimSpace(config.System.RecordFile); path != "" {
		if _, err := replay.OpenRecorder(path); err != nil {
			return nil, err
		}
	}

	pool := qpool.NewQ(ctx, 1, runtime.NumCPU()*4, qpool.NewConfig())
	qpool.SuppressLogging()

	if replayPath := strings.TrimSpace(config.System.ReplayFile); replayPath != "" {
		applyReplayMeta(replayPath)
	} else if universe := market.DiscoverSymbols(ctx, config.System.QuoteCurrency); len(universe) > 0 {
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

	if err := booter.AddSystems(
		pumpdump.NewSignal(runCtx, pool),
		correlation.NewSignal(runCtx, pool),
		depthflow.NewSignal(runCtx, pool),
		hawkes.NewSignal(runCtx, pool),
		leadlag.NewSignal(runCtx, pool),
		liquidity.NewSignal(runCtx, pool),
		sentiment.NewSignal(runCtx, pool),
		fluid.NewSignal(runCtx, pool),
		causal.NewSignal(runCtx, pool),
		cvd.NewSignal(runCtx, pool),
		toxicity.NewToxicity(runCtx, pool),
		exhaust.NewSignal(runCtx, pool),
		trader.NewCrypto(runCtx, pool, tradingWallet, tracker, config.Runtime),
		view.NewOHLC(runCtx, pool, tracker),
		view.NewGauges(runCtx, pool),
	); err != nil {
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

	return &engineResult{Wallet: tradingWallet}, nil
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

		payload, err := json.Marshal(map[string]any{
			"score_eur":   score - start,
			"equity_eur":  score,
			"wallet_eur":  start,
			"balance_eur": result.Wallet.BalanceCopy(),
			"open_bases":  len(result.Wallet.Snapshot().Inventory),
		})

		if err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		fmt.Println(string(payload))
	},
}

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Search tunable config against a replay fixture to maximize wallet equity",
	Run: func(cmd *cobra.Command, args []string) {
		replayFile, err := cmd.Flags().GetString("replay")

		if err != nil || strings.TrimSpace(replayFile) == "" {
			errnie.Error(fmt.Errorf("tune requires --replay"))
			os.Exit(1)
		}

		holdoutFiles, err := cmd.Flags().GetStringArray("holdout")

		if err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		stressHoldout, err := cmd.Flags().GetBool("stress-holdout")

		if err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		iterations, err := cmd.Flags().GetInt("iterations")

		if err != nil || iterations <= 0 {
			iterations = 32
		}

		workers, err := cmd.Flags().GetInt("workers")

		if err != nil || workers <= 0 {
			workers = runtime.NumCPU()
		}

		output, err := cmd.Flags().GetString("output")

		if err != nil || strings.TrimSpace(output) == "" {
			output = config.DefaultTunedPath()
		}

		maxGapFlag, err := cmd.Flags().GetFloat64("max-train-holdout-gap")

		if err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		maxTrainHoldoutGap := resolveMaxTrainHoldoutGap(maxGapFlag, config.System.WalletEUR)

		bestSelection := -1e18
		bestTrainScore := -1e18
		bestHoldoutScores := []float64(nil)
		bestGap := 0.0
		bestConfig := config.ExtractTunables(config.System)
		var rejectedOverfit atomic.Int64
		var bestMu sync.Mutex
		jobs := make(chan config.Tunables, workers*2)
		var waitGroup sync.WaitGroup

		for worker := 0; worker < workers; worker++ {
			waitGroup.Go(func() {
				for candidate := range jobs {
					scores, scoreErr := scoreTrial(
						replayFile, holdoutFiles, stressHoldout, maxTrainHoldoutGap, candidate,
					)

					if scoreErr != nil {
						errnie.Error(scoreErr)
						continue
					}

					if !scores.eligible {
						rejectedOverfit.Add(1)
						continue
					}

					bestMu.Lock()

					if scores.selection > bestSelection {
						bestSelection = scores.selection
						bestTrainScore = scores.trainScore
						bestHoldoutScores = append([]float64(nil), scores.holdoutScores...)
						bestGap = scores.gap
						bestConfig = candidate
					}

					bestMu.Unlock()
				}
			})
		}

		for trial := 0; trial < iterations; trial++ {
			jobs <- config.MutateTunables(config.System, nil)
		}

		close(jobs)
		waitGroup.Wait()

		overlay := bestConfig
		overlay.Apply(config.System)

		if err := config.SaveTunablesFile(output, config.System); err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		payload, _ := json.Marshal(map[string]any{
			"best_score_eur":         bestSelection,
			"best_train_eur":         bestTrainScore,
			"best_holdout_eur":       bestHoldoutScores,
			"best_train_holdout_gap": bestGap,
			"max_train_holdout_gap":  maxTrainHoldoutGap,
			"rejected_overfit":       rejectedOverfit.Load(),
			"holdout_replays":        holdoutFiles,
			"stress_holdout":         stressHoldout,
			"output":                 output,
			"iterations":             iterations,
			"workers":                workers,
		})
		fmt.Println(string(payload))
	},
}

func init() {
	rootCmd.AddCommand(evalCmd)
	tuneCmd.Flags().String("replay", "", "Replay JSONL fixture path")
	tuneCmd.Flags().StringArray("holdout", nil, "Holdout replay JSONL paths; best config is chosen by minimum holdout score")
	tuneCmd.Flags().Bool("stress-holdout", true, "Run holdout evals with execution stress enabled")
	tuneCmd.Flags().Float64("max-train-holdout-gap", 0, "Reject candidates when train minus min holdout exceeds this EUR (0 = 3% of wallet; -1 = disable)")
	tuneCmd.Flags().Int("iterations", 32, "Number of random trials")
	tuneCmd.Flags().Int("workers", runtime.NumCPU(), "Concurrent eval workers")
	tuneCmd.Flags().String("output", config.DefaultTunedPath(), "Path to write best tunables")
	rootCmd.AddCommand(tuneCmd)
}
