package signal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
	source   logic.SourceType
	signal   func(string, *logic.Entity) market.Signal
}

func NewSystem(
	ctx context.Context,
	pool *qpool.Q[any],
	source logic.SourceType,
	signal func(string, *logic.Entity) market.Signal,
) *System {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, err := telemetry.NewGauge(bus, source)

	if errnie.Error(err) != nil {
		cancel()
		return nil
	}

	return &System{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		bus:     bus,
		signals: sync.Map{},
		gauge:   gauge,
		source:  source,
		signal:  signal,
	}
}

func (system *System) Tick() error {
	for {
		message, err := system.bus.Receive("raw")

		if errnie.Error(err) != nil || message == nil {
			continue
		}

		var (
			ok      bool
			warmed  bool
			eventAt time.Time
			signal  market.Signal
		)

		switch message.Type {
		case "symbols":
			symbols, symbolOk := message.Value.([]string)

			if symbolOk {
				system.gauge.RegisterSymbols(symbols)
			}

			continue
		case "trades":
			var trades []*krakenmarket.TradeUpdate

			if trades, ok = message.Value.([]*krakenmarket.TradeUpdate); !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid trade",
				))

				continue
			}

			if len(trades) == 0 {
				continue
			}

			for _, trade := range trades {
				signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

				if signal == nil {
					errnie.Error(errors.New(
						string(system.source) + ": symbol not found",
					))

					continue
				}

				warmed = signal.Record(trade)
				eventAt = trade.Timestamp
			}
		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid ticker",
				))

				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New(
					string(system.source) + ": symbol not found",
				))

				continue
			}

			warmed = signal.Record(ticker)
			eventAt = ticker.Timestamp
		case "book":
			var book *krakenmarket.BookUpdate

			if book, ok = message.Value.(*krakenmarket.BookUpdate); !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid book",
				))

				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New(
					string(system.source) + ": symbol not found",
				))

				continue
			}

			warmed = signal.Record(book)
			eventAt = book.Timestamp
		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid feedback",
				))

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

		measurement, err := signal.Measure(system.feedback, eventAt)

		if errnie.Error(err) != nil {
			continue
		}

		if err := measurement.Publish(system.bus); errnie.Error(err) != nil {
			continue
		}

		errnie.Error(system.gauge.Publish(
			measurement,
			signal.Symbol(),
			warmed,
		))
	}
}

func (system *System) LoadSignal(
	entity logic.EntityType, symbol string,
) market.Signal {
	var (
		raw    any
		signal market.Signal
		ok     bool
	)

	mapKey := fmt.Sprintf("%d:%s", entity, symbol)

	raw, _ = system.signals.LoadOrStore(
		mapKey,
		system.signal(
			symbol,
			logic.NewEntity(entity),
		),
	)

	if signal, ok = raw.(market.Signal); !ok {
		errnie.Error(
			errors.New(string(system.source)+": symbol is not a Signal"),
			string(system.source)+": symbol is not a Signal",
		)

		return nil
	}

	return signal
}

func (system *System) Bus() *internal.Bus {
	return system.bus
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}
