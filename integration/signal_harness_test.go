package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/rawbus"
)

type signalRunner interface {
	Tick() error
	Close() error
}

type signalFactory func(context.Context, *qpool.Q[any]) signalRunner

type signalScenarioHarness struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q[any]
	feed         *internal.Bus
	subscriber   *internal.Bus
	runner       signalRunner
	measurements chan logic.Measurement
	errs         chan error
}

func newSignalScenarioHarness(
	test *testing.T,
	factory signalFactory,
) *signalScenarioHarness {
	test.Helper()
	configureSignalScenario()

	ctx, cancel := context.WithCancel(context.Background())
	pool := qpool.NewQ[any](ctx, 4, 128, nil)
	feed := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{
			internal.ChannelRaw,
			internal.ChannelMeasurements,
			internal.ChannelUI,
		},
		nil,
	)
	subscriber := internal.NewBus(
		ctx,
		pool,
		nil,
		[]internal.Subscription{
			internal.Subscribe(
				internal.ChannelMeasurements,
				"integration:signals:"+strings.ReplaceAll(test.Name(), "/", ":"),
			),
		},
	)
	runner := factory(ctx, pool)

	if runner == nil {
		cancel()
		pool.Close()
		test.Fatal("integration: signal system constructor returned nil")
	}

	harness := &signalScenarioHarness{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		feed:         feed,
		subscriber:   subscriber,
		runner:       runner,
		measurements: make(chan logic.Measurement, 64),
		errs:         make(chan error, 4),
	}

	go harness.collectMeasurements()
	go func() {
		if err := runner.Tick(); err != nil && !internal.IsShutdown(err) && ctx.Err() == nil {
			harness.errs <- err
		}
	}()

	test.Cleanup(func() {
		cancel()
		_ = runner.Close()
		_ = subscriber.Close()
		_ = feed.Close()
		pool.Close()
	})

	return harness
}

func configureSignalScenario() {
	viper.Set("system.queue.ttl", time.Second)
	viper.Set("system.queue.buffer", 128)
	viper.Set("telemetry.gauge.readings_capacity", 32)
	viper.Set("market.book_depth_levels", 10)
	viper.Set("market.anchor_symbol", "BTC/EUR")
	viper.Set("market.default_symbols", []string{"BTC/EUR"})
	viper.Set("signals.trade_match_window", time.Minute)
	viper.Set("signals.cross_section.return_capacity", 64)
	viper.Set("signals.cross_section.min_bars", 2)
	viper.Set("signals.cross_section.breadth_history_capacity", 8)
	viper.Set("signals.causal.measurements_capacity", 8)
	viper.Set("signals.causal.alpha", 0.5)
	viper.Set("signals.causal.surprise_threshold", 1.0)
	viper.Set("signals.correlation.measurements_capacity", 8)
	viper.Set("signals.cvd.measurements_capacity", 8)
	viper.Set("signals.depthflow.measurements_capacity", 8)
	viper.Set("signals.exhaust.measurements_capacity", 8)
	viper.Set("signals.exhaust.history_capacity", 24)
	viper.Set("signals.fluid.measurements_capacity", 8)
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("signals.hawkes.measurements_capacity", 128)
	viper.Set("signals.hawkes_fit_cooldown", 0)
	viper.Set("signals.leadlag.measurements_capacity", 8)
	viper.Set("signals.liquidity.measurements_capacity", 8)
	viper.Set("signals.manifold.measurements_capacity", 16)
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 16)
	viper.Set("signals.manifold.grid_x", 32)
	viper.Set("signals.manifold.grid_y", 3)
	viper.Set("signals.manifold.grid_z", 16)
	viper.Set("signals.manifold.max_modes", 32)
	viper.Set("signals.manifold.integration_interval", 100*time.Millisecond)
	viper.Set("signals.prediction.measurements_capacity", 8)
	viper.Set("signals.pumpdump.measurements_capacity", 8)
	viper.Set("signals.pumpdump.window", time.Minute)
	viper.Set("signals.pumpdump.volume.epsilon", 0)
	viper.Set("signals.pumpdump.surprise.matrix.alpha", 0.5)
	viper.Set("signals.pumpdump.surprise.weights.threshold", 0.5)
	viper.Set("signals.sentiment.measurements_capacity", 8)
	viper.Set("signals.toxicity.measurements_capacity", 8)
	viper.Set("story.prediction.horizon", time.Minute)
	viper.Set("story.prediction.interval", 0)
}

func (harness *signalScenarioHarness) collectMeasurements() {
	for {
		if harness.ctx.Err() != nil {
			return
		}

		row, err := harness.subscriber.Receive(internal.ChannelMeasurements)

		if internal.IsShutdown(err) || harness.ctx.Err() != nil {
			return
		}

		if err != nil {
			harness.errs <- err
			return
		}

		if row == nil {
			continue
		}

		measurement, ok := row.Value.(logic.Measurement)

		if !ok {
			continue
		}

		harness.measurements <- measurement
	}
}

func (harness *signalScenarioHarness) publishTrades(
	test *testing.T,
	trades []*krakenmarket.TradeUpdate,
) {
	test.Helper()

	updates := krakenmarket.TradeUpdates(trades)

	if err := rawbus.Send(harness.feed, rawbus.TypeTrade, &updates); err != nil {
		test.Fatal(err)
	}
}

func (harness *signalScenarioHarness) publishTickers(
	test *testing.T,
	tickers []*krakenmarket.TickerUpdate,
) {
	test.Helper()

	updates := krakenmarket.TickerUpdates(tickers)

	if err := rawbus.Send(harness.feed, rawbus.TypeTicker, &updates); err != nil {
		test.Fatal(err)
	}
}

func (harness *signalScenarioHarness) publishBooks(
	test *testing.T,
	books []*krakenmarket.BookUpdate,
) {
	test.Helper()

	updates := krakenmarket.BookUpdates(books)

	if err := rawbus.Send(harness.feed, rawbus.TypeBook, &updates); err != nil {
		test.Fatal(err)
	}
}

func (harness *signalScenarioHarness) publishMeasurement(
	test *testing.T,
	measurement logic.Measurement,
) {
	test.Helper()

	if err := harness.feed.Send(
		internal.ChannelMeasurements,
		rawbus.TypeMeasurements.String(),
		measurement,
	); err != nil {
		test.Fatal(err)
	}
}

func (harness *signalScenarioHarness) awaitMeasurement(
	test *testing.T,
	label string,
	matches func(logic.Measurement) bool,
) logic.Measurement {
	test.Helper()

	deadline := time.After(3 * time.Second)
	observed := make([]string, 0)

	for {
		select {
		case measurement := <-harness.measurements:
			observed = append(observed, fmt.Sprintf(
				"%s:%s:%s",
				measurement.Source,
				measurement.Symbol,
				measurement.Category,
			))

			if matches(measurement) {
				return measurement
			}
		case err := <-harness.errs:
			test.Fatalf("%s: signal tick failed: %v", label, err)
		case <-deadline:
			test.Fatalf("%s: timed out awaiting measurement; observed=%v", label, observed)
		}
	}
}

func hasSignalCategory(
	source logic.SourceType,
	symbol string,
	category logic.CategoryType,
) func(logic.Measurement) bool {
	return func(measurement logic.Measurement) bool {
		return measurement.Source == source &&
			measurement.Symbol == symbol &&
			measurement.Category == category &&
			measurement.Publishable()
	}
}

func makeTrade(
	symbol string,
	side string,
	price float64,
	qty float64,
	at time.Time,
) *krakenmarket.TradeUpdate {
	return &krakenmarket.TradeUpdate{
		Symbol:    symbol,
		Side:      side,
		Price:     price,
		Qty:       qty,
		Timestamp: at,
	}
}

func makeTicker(
	symbol string,
	price float64,
	volume float64,
	changePct float64,
	at time.Time,
) *krakenmarket.TickerUpdate {
	return &krakenmarket.TickerUpdate{
		Symbol:    symbol,
		Last:      price,
		High:      price + 0.5,
		Low:       price - 0.5,
		Volume:    volume,
		VWAP:      price,
		ChangePct: changePct,
		Bid:       price - 0.01,
		Ask:       price + 0.01,
		BidQty:    1,
		AskQty:    1,
		Timestamp: at,
	}
}

func makeBook(
	symbol string,
	bidPrice float64,
	bidQty float64,
	askPrice float64,
	askQty float64,
	at time.Time,
) *krakenmarket.BookUpdate {
	return &krakenmarket.BookUpdate{
		Symbol:    symbol,
		Type:      "snapshot",
		Timestamp: at,
		Bids: []krakenmarket.BookLevel{
			{Price: bidPrice, Qty: bidQty},
			{Price: bidPrice - 0.01, Qty: bidQty * 0.5},
		},
		Asks: []krakenmarket.BookLevel{
			{Price: askPrice, Qty: askQty},
			{Price: askPrice + 0.01, Qty: askQty * 0.5},
		},
	}
}
