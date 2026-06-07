package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/focus"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives/types"
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
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/trader"
)

const scenarioRunTimeout = 12 * time.Second

/*
Harness wires the production stack with kraken/replay market data and paper trading.
*/
type Harness struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q[any]
	engine     *cmd.Engine
	replay     *paper.WebSocket
	capture    io.Reader
	tape       *Tape
	streams    *focus.Set
	measureBus *qpool.BroadcastGroup
	auditPath  string
}

func NewHarness(parent context.Context, capture io.Reader, auditPath string) (*Harness, error) {
	ctx, cancel := context.WithCancel(parent)

	pool := qpool.NewQ[any](ctx, 2, 8, qpool.NewConfig())

	engine, err := cmd.NewEngine(ctx, pool)

	if err != nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: engine: %w", err)
	}

	systemCtx := engine.Context()
	toxicity.ResetDefault()

	replaySocket := paper.NewWebSocket(systemCtx, pool)

	streams := focus.NewSet()
	crypto := trader.NewCrypto(systemCtx, pool, streams)
	story := market.NewStory(systemCtx, pool)
	story.SetPositionHeld(crypto.SymbolHeld)

	if err := engine.AddSystems(
		replaySocket,
		krakenmarket.NewInstrument(systemCtx, pool),
		causal.NewSignal(systemCtx, pool),
		correlation.NewSignal(systemCtx, pool),
		cvd.NewSignal(systemCtx, pool),
		depthflow.NewSignal(systemCtx, pool),
		exhaust.NewSignal(systemCtx, pool),
		fluid.NewSignal(systemCtx, pool),
		hawkes.NewSignal(systemCtx, pool),
		leadlag.NewSignal(systemCtx, pool),
		liquidity.NewSignal(systemCtx, pool),
		pumpdump.NewSignal(systemCtx, pool),
		sentiment.NewSignal(systemCtx, pool),
		toxicity.NewToxicity(systemCtx, pool),
		story,
		crypto,
	); err != nil {
		cancel()
		pool.Close()

		return nil, fmt.Errorf("integration harness: register systems: %w", err)
	}

	tape := NewTape()
	tape.Subscribe(ctx, pool)

	harness := &Harness{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		engine:     engine,
		replay:     replaySocket,
		capture:    capture,
		tape:       tape,
		streams:    streams,
		measureBus: bus.Group(pool, "measurements", 10*time.Millisecond),
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

	harness.publishHeldPositions(scenario.HoldingSymbols)

	replayDone := make(chan error, 1)

	go func() {
		replayDone <- harness.playCapture()
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
	closeErr := harness.engine.Close()
	harness.cancel()

	select {
	case <-engineDone:
	case <-time.After(2 * time.Second):
		report.Checks = append(report.Checks, CheckResult{
			ID:     "engine.shutdown",
			Name:   "Engine stops after scenario cancel",
			Pass:   false,
			Detail: "timed out waiting for engine shutdown",
		})
	}

	if closeErr != nil {
		report.Checks = append(report.Checks, CheckResult{
			ID:     "engine.close",
			Name:   "Engine close completed without non-context errors",
			Pass:   false,
			Detail: closeErr.Error(),
		})
	}

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

type captureFrame struct {
	Channel string
	Type    string
	Data    json.RawMessage
}

func (harness *Harness) playCapture() error {
	reader := bufio.NewReader(harness.capture)
	raw := bus.Group(harness.pool, "raw", 10*time.Millisecond)

	for {
		line, readErr := reader.ReadBytes('\n')

		if len(line) > 0 {
			frame, err := decodeCaptureFrame(line)

			if err != nil {
				return err
			}

			group := bus.Group(harness.pool, frame.Channel, 10*time.Millisecond)
			envelope := map[string]any{
				"channel": frame.Channel,
				"type":    frame.Type,
				"data":    frame.Data,
			}

			group.Send(&qpool.QValue[any]{
				Type:  frame.Channel,
				Value: envelope,
			})
			raw.Send(&qpool.QValue[any]{
				Type:  frame.Channel,
				Value: envelope,
			})
			harness.sleep(2 * time.Millisecond)
		}

		if readErr == nil {
			continue
		}

		if errors.Is(readErr, io.EOF) {
			break
		}

		return fmt.Errorf("integration capture: read: %w", readErr)
	}

	return nil
}

func decodeCaptureFrame(line []byte) (captureFrame, error) {
	raw := make(map[string]json.RawMessage)

	if err := json.Unmarshal(line, &raw); err != nil {
		return captureFrame{}, fmt.Errorf("integration capture: decode frame: %w", err)
	}

	frame := captureFrame{}

	if err := json.Unmarshal(raw["channel"], &frame.Channel); err != nil {
		return captureFrame{}, fmt.Errorf("integration capture: channel: %w", err)
	}

	if err := json.Unmarshal(raw["type"], &frame.Type); err != nil {
		return captureFrame{}, fmt.Errorf("integration capture: type: %w", err)
	}

	data, err := decodeCaptureData(raw["data"])

	if err != nil {
		return captureFrame{}, err
	}

	frame.Data = data

	return frame, nil
}

func decodeCaptureData(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("integration capture: data is required")
	}

	if raw[0] != '"' {
		return append(json.RawMessage(nil), raw...), nil
	}

	var decoded []byte

	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("integration capture: data bytes: %w", err)
	}

	return json.RawMessage(decoded), nil
}

func (harness *Harness) publishDirectMeasurements(rows []types.Measurement) {
	for _, row := range rows {
		harness.measureBus.Send(&qpool.QValue[any]{Value: row})
		harness.sleep(5 * time.Millisecond)
	}
}

func (harness *Harness) publishHeldPositions(symbols []string) {
	if len(symbols) == 0 {
		return
	}

	holdings := make([]map[string]any, 0, len(symbols))

	for _, symbol := range symbols {
		holdings = append(holdings, map[string]any{
			"symbol": symbol,
			"qty":    1.0,
		})
	}

	raw := bus.Group(harness.pool, "raw", 10*time.Millisecond)
	raw.Send(&qpool.QValue[any]{
		Type: "holdings",
		Value: map[string]any{
			"channel":  "holdings",
			"holdings": holdings,
		},
	})
	harness.sleep(50 * time.Millisecond)
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
	harness.sleep(timeout)
}

func ConfigureViper(auditPath string) {
	profilePath := filepath.Join(os.TempDir(), "symm-integration-latency.json")
	viper.Set("trading.paper.latency_profile", profilePath)

	viper.Set("trading.model", "paper")
	viper.Set("trading.record.file", "")
	viper.Set("trading.paper.wallet_eur", 200.0)
	viper.Set("trading.paper.maker_fee_pct", 0.001)
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
	viper.Set("market.l3_enabled", false)
	viper.Set("trading.audit.file", auditPath)
	viper.Set("trading.audit.gate_cooldown", time.Nanosecond)
	viper.Set("trading.paper.default_one_way_latency", time.Nanosecond)
	viper.Set("trading.order_ack_timeout", 5*time.Second)
	viper.Set("signals.raw_dump_dir", filepath.Dir(auditPath))
}
