package causal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	symmring "github.com/theapemachine/symm/ring"
	"github.com/theapemachine/symm/telemetry"
)

type System struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q[any]
	bus             *internal.Bus
	signals         sync.Map
	symbols         sync.Map
	gauge           *telemetry.Gauge
	feedback        *market.Feedback
	crossSection    *crossSection
	contagionSpread symmring.FloatRing
	lastPublish     time.Time
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceCausal)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	return &System{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		bus:          bus,
		signals:      sync.Map{},
		symbols:      sync.Map{},
		gauge:        gauge,
		crossSection: &crossSection{},
	}
}

func (system *System) Tick() error {
	for {
		message, err := system.bus.Receive("raw")

		if errnie.Error(err) != nil || message == nil {
			continue
		}

		var (
			signal *Signal
			ok     bool
			warmed bool
		)

		switch message.Type {
		case "symbols":
			symbols, symbolOk := message.Value.([]string)
			if symbolOk {
				system.gauge.RegisterSymbols(symbols)
			}
			continue
		case "trades":
			var trade *krakenmarket.TradeUpdate

			if trade, ok = message.Value.(*krakenmarket.TradeUpdate); !ok {
				errnie.Error(errors.New("causal: invalid trade"), "causal: invalid trade")
				continue
			}

			system.feedTrade(trade)

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("causal: symbol not found"), "causal: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("causal: invalid ticker"), "causal: invalid ticker")
				continue
			}

			system.feedTicker(ticker)

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("causal: symbol not found"), "causal: symbol not found")
				continue
			}

			warmed = signal.Record(ticker)

		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("causal: invalid book"), "causal: invalid book")
				continue
			}

			system.feedBook(book)

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("causal: symbol not found"), "causal: symbol not found")
				continue
			}

			warmed = signal.Record(book)

		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("causal: invalid feedback"), "causal: invalid feedback")
				continue
			}

			system.feedback = feedback
			continue
		default:
			continue
		}

		if signal == nil {
			continue
		}

		eventAt, eventErr := krakenmarket.EventTimeFromBus(message.Type, message.Value)

		if errnie.Error(eventErr) != nil {
			continue
		}

		measurement, measureErr := signal.Measure(system.feedback, eventAt)

		if errnie.Error(measureErr) != nil {
			continue
		}

		if publishErr := measurement.Publish(system.bus); errnie.Error(publishErr) != nil {
			continue
		}

		errnie.Error(system.gauge.Publish(
			measurement,
			signal.symbol,
			warmed,
		))
	}
}

func (system *System) feedTrade(trade *krakenmarket.TradeUpdate) {
	state := system.loadSymbol(trade.Symbol)

	if err := state.FeedTrade(*trade); err != nil {
		errnie.Error(err)
	}
}

func (system *System) feedTicker(ticker *krakenmarket.TickerUpdate) {
	state := system.loadSymbol(ticker.Symbol)
	state.FeedTicker(*ticker)
	system.crossSection.publishChangePct(ticker.Symbol, ticker.ChangePct)
}

func (system *System) feedBook(book *krakenmarket.Book) {
	state := system.loadSymbol(book.Symbol)
	state.FeedBook(*book)
}

func (system *System) loadSymbol(symbol string) *CausalSymbol {
	raw, _ := system.symbols.LoadOrStore(symbol, NewCausalSymbol())

	state, ok := raw.(*CausalSymbol)

	if !ok {
		return nil
	}

	return state
}

func (system *System) shouldPublish(now time.Time) bool {
	interval := viper.GetDuration("signals.causal.publish_interval")

	if interval <= 0 {
		interval = time.Second
	}

	if now.Sub(system.lastPublish) < interval {
		return false
	}

	system.lastPublish = now

	return true
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.causal.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.causal.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.causal.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	mapKey := fmt.Sprintf("%d:%s", entity, symbol)

	raw, _ = system.signals.LoadOrStore(
		mapKey, NewSignal(
			symbol,
			logic.NewEntity(entity),
			measurementsCapacity,
			system,
			threshold,
			alpha,
		),
	)

	if signal, ok = raw.(*Signal); !ok {
		errnie.Error(errors.New("causal: symbol is not a Signal"), "causal: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}
