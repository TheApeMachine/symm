package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/focus"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/paper"
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
	replay     *paper.WebSocket
	relay      *RawRelay
	tape       *Tape
	streams    *focus.Set
	measureBus *qpool.BroadcastGroup
	auditPath  string
}

func NewHarness(parent context.Context, capture io.Reader, auditPath string) (*Harness, error) {
	ctx, cancel := context.WithCancel(parent)

	pool := qpool.NewQ(ctx, 2, 8, qpool.NewConfig())

	replaySocket := paper.NewWebSocket(ctx, pool)
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
	story := market.NewStory(ctx, pool, streams)
	crypto := trader.NewCrypto(ctx, pool, streams)

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
		auditPath:  auditPath,
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

	var engineErr error
	engineDone := make(chan struct{})

	go func() {
		engineErr = harness.engine.Start()
		close(engineDone)
	}()

	harness.sleep(150 * time.Millisecond)

	for _, symbol := range scenario.HoldingSymbols {
		harness.streams.Add(symbol)
	}

	replayDone := make(chan error, 1)

	go func() {
		replayDone <- harness.replay.Tick()
	}()

	runTimeout := scenarioRunTimeout

	if scenario.RunTimeout > 0 {
		runTimeout = scenario.RunTimeout
	}

	replayDeadline := time.After(runTimeout)

	select {
	case replayErr := <-replayDone:
		if replayErr != nil {
			report.Checks = append(report.Checks, CheckResult{
				ID:     "replay.tick",
				Name:   "Replay capture playback",
				Pass:   false,
				Detail: replayErr.Error(),
			})
		}
	case <-replayDeadline:
		report.Checks = append(report.Checks, CheckResult{
			ID:     "replay.tick",
			Name:   "Replay capture playback",
			Pass:   false,
			Detail: "timed out waiting for replay playback",
		})
	}

	postDelay := scenario.PostReplayDelay

	if postDelay <= 0 {
		postDelay = 300 * time.Millisecond
	}

	harness.sleep(postDelay)

	pace := scenario.PostReplayPace

	if pace <= 0 {
		pace = 50 * time.Millisecond
	}

	for _, ticker := range scenario.PostReplayTickers {
		harness.InjectTicker(ticker)
		harness.sleep(pace)
	}

	if len(scenario.DirectMeasurements) > 0 {
		for _, ticker := range scenario.PreDirectTickers {
			harness.InjectTicker(ticker)
			harness.sleep(pace)
		}

		for _, book := range scenario.PreDirectBooks {
			harness.InjectBook(book)
			harness.sleep(pace)
		}

		harness.waitDeskReady(3 * time.Second)
		harness.publishDirectMeasurements(scenario.DirectMeasurements)

		postOrderDelay := scenario.PostOrderDelay

		if postOrderDelay <= 0 {
			postOrderDelay = 500 * time.Millisecond
		}

		if len(scenario.PostReplayTrades) > 0 ||
			len(scenario.PostReplayTradeBatches) > 0 ||
			len(scenario.PostOrderTickers) > 0 {
			harness.sleep(postOrderDelay)
		}
	}

	for _, batch := range scenario.PostReplayTradeBatches {
		for _, trade := range batch {
			harness.InjectTrade(trade)
		}

		harness.sleep(pace)
	}

	for _, book := range scenario.PostOrderBooks {
		harness.InjectBook(book)
		harness.sleep(pace)
	}

	for _, ticker := range scenario.PostOrderTickers {
		harness.InjectTicker(ticker)
		harness.sleep(pace)
	}

	for _, trade := range scenario.PostReplayTrades {
		harness.InjectTrade(trade)
		harness.sleep(pace)
	}

	settleDelay := scenario.SettleDelay

	if settleDelay <= 0 {
		settleDelay = 400 * time.Millisecond
	}

	harness.sleep(settleDelay)
	harness.cancel()
	<-engineDone

	snapshot := harness.tape.Snapshot(harness.auditPath)

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
		harness.sleep(5 * time.Millisecond)
	}
}

func (harness *Harness) sleep(duration time.Duration) {
	if duration <= 0 {
		return
	}

	select {
	case <-harness.ctx.Done():
	case <-time.After(duration):
	}
}

func (harness *Harness) waitDeskReady(timeout time.Duration) {
	deadline := time.Now().Add(timeout)

	for !trading.DeskReady() {
		if time.Now().After(deadline) {
			return
		}

		harness.sleep(10 * time.Millisecond)
	}
}

func ConfigureViper(auditPath string) {
	profilePath := filepath.Join(os.TempDir(), "symm-integration-latency.json")
	viper.Set("trading.paper.latency_profile", profilePath)

	viper.Set("trading.model", "paper")
	viper.Set("trading.record.file", "")
	viper.Set("trading.paper.wallet_eur", 200.0)
	viper.Set("trading.paper.maker_fee_pct", paper.DefaultMakerFeePct)
	viper.Set("market.perspectives.fixture_playbook", true)
	viper.Set("market.quote_currency", "EUR")
	viper.Set("market.max_scan_symbols", 8)
	viper.Set("market.symbols", []string{
		testSymbolPrimary,
		testSymbolSecondary,
		testSymbolLeader,
	})
	viper.Set("market.anchor_symbol", testSymbolLeader)
	viper.Set("market.default_symbols", []string{testSymbolLeader})
	viper.Set("trading.audit.file", auditPath)
	viper.Set("trading.audit.gate_cooldown", time.Nanosecond)
	viper.Set("trading.paper.default_one_way_latency", time.Nanosecond)
	viper.Set("trading.order_ack_timeout", 5*time.Second)
}

func resetTradingReady() {
	trading.ResetDeskReady()
	broker.ResetQuoteCacheForTest()
}
