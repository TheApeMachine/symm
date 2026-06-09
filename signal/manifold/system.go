package manifold

import (
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
	pool     *qpool.Q[any]
	bus      *internal.Bus
	signals  sync.Map
	gauge    *telemetry.Gauge
	feedback *market.Feedback
	field    *Field
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceManifold)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	field, fieldErr := newField()

	if fieldErr != nil {
		cancel()
		errnie.Error(fieldErr)

		return nil
	}

	return &System{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		bus:     bus,
		signals: sync.Map{},
		gauge:   gauge,
		field:   field,
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
				system.field.RegisterSymbols(symbols)
			}

			continue
		case "trades":
			var trade *krakenmarket.TradeUpdate

			if trade, ok = message.Value.(*krakenmarket.TradeUpdate); !ok {
				errnie.Error(errors.New("manifold: invalid trade"), "manifold: invalid trade")
				continue
			}

			eventAt, eventErr := krakenmarket.EventTimeFromTrade(trade)

			if errnie.Error(eventErr) != nil {
				continue
			}

			if feedErr := system.field.FeedTrade(trade, eventAt); feedErr != nil {
				errnie.Error(feedErr)
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("manifold: symbol not found"), "manifold: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("manifold: invalid ticker"), "manifold: invalid ticker")
				continue
			}

			eventAt, eventErr := krakenmarket.EventTimeFromTicker(ticker)

			if errnie.Error(eventErr) != nil {
				continue
			}

			if feedErr := system.field.FeedTicker(*ticker, eventAt); feedErr != nil {
				errnie.Error(feedErr)
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("manifold: symbol not found"), "manifold: symbol not found")
				continue
			}

			warmed = signal.Record(ticker)

		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("manifold: invalid book"), "manifold: invalid book")
				continue
			}

			eventAt, eventErr := krakenmarket.EventTimeFromBook(book)

			if errnie.Error(eventErr) != nil {
				continue
			}

			if feedErr := system.field.FeedBook(*book, eventAt); feedErr != nil {
				errnie.Error(feedErr)
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("manifold: symbol not found"), "manifold: symbol not found")
				continue
			}

			warmed = signal.Record(book)

		case "futures_book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("manifold: invalid futures book"), "manifold: invalid futures book")
				continue
			}

			eventAt, eventErr := krakenmarket.EventTimeFromBook(book)

			if errnie.Error(eventErr) != nil {
				continue
			}

			if feedErr := system.field.FeedFuturesBook(*book, eventAt); feedErr != nil {
				errnie.Error(feedErr)
			}

			spotSymbol := system.field.SpotSymbolForIdentity(book.InstrumentIdentity())

			if spotSymbol == "" {
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, spotSymbol)

			if signal == nil {
				errnie.Error(errors.New("manifold: symbol not found"), "manifold: symbol not found")
				continue
			}

			warmed = signal.Record(book)

		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("manifold: invalid feedback"), "manifold: invalid feedback")
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

		errnie.Error(system.publishSnapshot(eventAt))
	}
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.manifold.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.manifold.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.manifold.measurements_capacity")

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
		errnie.Error(errors.New("manifold: symbol is not a Signal"), "manifold: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()

	if system.field != nil {
		system.field.Close()
	}

	return system.bus.Close()
}
