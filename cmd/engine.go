package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"

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
	if config.System.Headless {
		config.System.ReplayPace = 0
	}

	market.ResetBookHealth()
	broker.ResetStressMachine()

	pool := qpool.NewQ(ctx, 1, runtime.NumCPU()*4, qpool.NewConfig())
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
