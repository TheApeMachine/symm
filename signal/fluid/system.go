package fluid

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/telemetry"
)

type System struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	pool     *qpool.Q[any]
	bus      *internal.Bus
	signals  sync.Map
	symbols  sync.Map
	gauge    *telemetry.Gauge
	feedback *market.Feedback
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceFluid)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	return &System{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		bus:     bus,
		signals: sync.Map{},
		symbols: sync.Map{},
		gauge:   gauge,
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
		)

		switch message.Type {
		case "trades":
			var trade *krakenmarket.TradeUpdate

			if trade, ok = message.Value.(*krakenmarket.TradeUpdate); !ok {
				errnie.Error(errors.New("fluid: invalid trade"), "fluid: invalid trade")
				continue
			}

			system.feedTrade(trade)

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("fluid: symbol not found"), "fluid: symbol not found")
				continue
			}

			signal.measurements.Value = trade
			signal.measurements = signal.measurements.Next()
		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("fluid: invalid ticker"), "fluid: invalid ticker")
				continue
			}

			if feedErr := system.feedTicker(ticker); feedErr != nil {
				errnie.Error(feedErr)
				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("fluid: symbol not found"), "fluid: symbol not found")
				continue
			}

			signal.measurements.Value = ticker
			signal.measurements = signal.measurements.Next()
		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("fluid: invalid book"), "fluid: invalid book")
				continue
			}

			if feedErr := system.feedBook(book); feedErr != nil {
				errnie.Error(feedErr)
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("fluid: symbol not found"), "fluid: symbol not found")
				continue
			}

			signal.measurements.Value = book
			signal.measurements = signal.measurements.Next()
		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("fluid: invalid feedback"), "fluid: invalid feedback")
				continue
			}

			system.feedback = feedback
			continue
		default:
			continue
		}

		measurement, measureErr := signal.Measure(system.feedback)

		if errnie.Error(measureErr) != nil {
			continue
		}

		system.bus.Send(
			"measurements",
			"measurements",
			measurement,
		)

		errnie.Error(system.gauge.Publish(measurement))
	}
}

func (system *System) feedTrade(trade *krakenmarket.TradeUpdate) {
	state := system.loadSymbol(trade.Symbol)

	if err := state.FeedTradeSide(trade.Timestamp, trade.Qty, trade.Side); err != nil {
		errnie.Error(err)
	}
}

func (system *System) feedTicker(ticker *krakenmarket.TickerUpdate) error {
	state := system.loadSymbol(ticker.Symbol)

	return state.FeedTicker(*ticker)
}

func (system *System) feedBook(book *krakenmarket.Book) error {
	state := system.loadSymbol(book.Symbol)

	return state.FeedBook(*book)
}

func (system *System) loadSymbol(symbol string) *FluidSymbol {
	raw, _ := system.symbols.LoadOrStore(symbol, mustNewFluidSymbol(symbol))

	state, ok := raw.(*FluidSymbol)

	if !ok {
		return nil
	}

	return state
}

func mustNewFluidSymbol(symbol string) *FluidSymbol {
	state, err := NewFluidSymbol(symbol)

	if err != nil {
		panic(err)
	}

	return state
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.fluid.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.fluid.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.fluid.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	mapKey := fmt.Sprintf("%d:%s", entity, symbol)

	raw, _ = system.signals.LoadOrStore(
		mapKey, NewSignal(
			symbol,
			logic.NewEntity(entity),
			ring.New(measurementsCapacity),
			system,
			threshold,
			alpha,
		),
	)

	if signal, ok = raw.(*Signal); !ok {
		errnie.Error(errors.New("fluid: symbol is not a Signal"), "fluid: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}
