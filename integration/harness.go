package integration

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/focus"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/replay"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
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
)

const scenarioRunTimeout = 12 * time.Second

/*
Harness wires the production stack with kraken/replay market data and paper trading.
*/
type Harness struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q
	engine     *cmd.Engine
	replay     *replay.WebSocket
	relay      *RawRelay
	tape       *Tape
	streams    *focus.Set
	measureBus *qpool.BroadcastGroup
}

func NewHarness(parent context.Context, capture io.Reader) (*Harness, error) {
	ctx, cancel := context.WithCancel(parent)

	pool := qpool.NewQ(ctx, 2, 8, qpool.NewConfig())

	replaySocket, err := replay.NewWebSocket(ctx, pool, capture)

	if err != nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: replay websocket: %w", err)
	}

	relay := NewRawRelay(ctx, pool)

	if relay == nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: raw relay")
	}

	engine, err := cmd.NewEngine(ctx, pool)

	if err != nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: engine: %w", err)
	}

	streams := focus.NewSet()
	story, storyErr := market.NewStory(ctx, pool, streams)

	if storyErr != nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: story: %w", storyErr)
	}

	crypto, cryptoErr := trader.NewCrypto(ctx, pool, streams)

	if cryptoErr != nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: crypto: %w", cryptoErr)
	}

	if err := engine.AddSystems(
		relay,
		replaySocket,
		krakenmarket.NewInstrument(ctx, pool),
		causal.NewSignal(ctx, pool),
		correlation.NewSignal(ctx, pool),
		cvd.NewSignal(ctx, pool),
		depthflow.NewSignal(ctx, pool),
		exhaust.NewSignal(ctx, pool),
		fluid.NewSignal(ctx, pool),
		hawkes.NewSignal(ctx, pool),
		leadlag.NewSignal(ctx, pool),
		liquidity.NewSignal(ctx, pool),
		pumpdump.NewSignal(ctx, pool),
		sentiment.NewSignal(ctx, pool),
		toxicity.NewToxicity(ctx, pool),
		story,
		crypto,
		private.NewWebSocket(ctx, pool, "", ""),
	); err != nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: register systems: %w", err)
	}

	tape := NewTape()
	tape.Subscribe(pool)

	harness := &Harness{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		engine:     engine,
		replay:     replaySocket,
		relay:      relay,
		tape:       tape,
		streams:    streams,
		measureBus: pool.CreateBroadcastGroup("measurements", 10*time.Millisecond),
	}

	return harness, nil
}

func (harness *Harness) Close() {
	harness.cancel()
	harness.pool.Close()
}

/*
RunScenario replays synthetic capture data and evaluates checks.
*/
func (harness *Harness) RunScenario(scenario Scenario) ScenarioReport {
	started := time.Now()
	report := ScenarioReport{
		ID:        scenario.ID,
		Name:      scenario.Name,
		StartedAt: started,
		Checks:    make([]CheckResult, 0, len(scenario.Checks)),
	}

	runCtx, cancel := context.WithTimeout(harness.ctx, scenarioRunTimeout)
	defer cancel()

	var engineErr error
	engineDone := make(chan struct{})

	go func() {
		engineErr = harness.engine.Start()
		close(engineDone)
	}()

	time.Sleep(150 * time.Millisecond)
	harness.publishDirectMeasurements(scenario.DirectMeasurements)

	replayDone := make(chan error, 1)

	go func() {
		replayDone <- harness.replay.Tick()
	}()

	select {
	case replayErr := <-replayDone:
		if replayErr != nil && runCtx.Err() == nil {
			report.Checks = append(report.Checks, CheckResult{
				ID:     "replay.tick",
				Name:   "Replay capture playback",
				Pass:   false,
				Detail: replayErr.Error(),
			})
		}
	case <-runCtx.Done():
		report.Checks = append(report.Checks, CheckResult{
			ID:     "replay.tick",
			Name:   "Replay capture playback",
			Pass:   false,
			Detail: runCtx.Err().Error(),
		})
	}

	settle := scenario.SettleDelay

	if settle <= 0 {
		settle = 400 * time.Millisecond
	}

	time.Sleep(settle)
	cancel()
	<-engineDone

	snapshot := harness.tape.Snapshot()

	for _, check := range scenario.Checks {
		pass, detail, contextMap := check.Evaluate(snapshot, engineErr)
		report.Checks = append(report.Checks, CheckResult{
			ID:      check.ID,
			Name:    check.Name,
			Pass:    pass,
			Detail:  detail,
			Context: contextMap,
		})
	}

	report.Pass = true

	for _, check := range report.Checks {
		if !check.Pass {
			report.Pass = false
		}
	}

	report.Elapsed = time.Since(started)

	return report
}

func (harness *Harness) publishDirectMeasurements(rows []perspectives.Measurement) {
	for _, row := range rows {
		harness.measureBus.Send(&qpool.QValue[any]{Value: row})
	}
}

/*
ConfigureViper sets paper/replay-friendly defaults for integration runs.
*/
func ConfigureViper(auditPath string) {
	viper.Set("trading.model", "paper")
	viper.Set("trading.record.file", "")
	viper.Set("trading.paper.wallet_eur", 200.0)
	viper.Set("market.quote_currency", "EUR")
	viper.Set("market.max_scan_symbols", 8)
	viper.Set("market.symbols", []string{
		testSymbolPrimary,
		testSymbolSecondary,
		testSymbolLeader,
	})
	viper.Set("trading.audit.file", auditPath)
}

func resetTradingReady() {
	trading.ResetDeskReady()
}
