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
	"github.com/theapemachine/symm/rawbus"
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
		[]internal.Channel{internal.ChannelMeasurements, internal.ChannelUI},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelRaw, "signal:"+string(source)),
		},
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
		message, err := system.bus.Receive(internal.ChannelRaw)

		if errnie.Error(err) != nil || message == nil {
			continue
		}

		switch rawbus.TypeFrom(message.Type) {
		case rawbus.TypeSymbols:
			symbols, symbolOk := message.Value.([]string)

			if symbolOk {
				system.gauge.RegisterSymbols(symbols)
			}

			continue
		case rawbus.TypeTrade:
			trades, ok := tradeUpdates(message.Value)

			if !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid trade",
				))

				continue
			}

			for _, trade := range trades {
				if trade == nil {
					continue
				}

				signal := system.LoadSignal(logic.EntityTrade, trade.Symbol)

				if signal == nil {
					errnie.Error(errors.New(
						string(system.source) + ": symbol not found",
					))

					continue
				}

				system.publishMeasurement(
					signal,
					signal.Record(trade),
					trade.Timestamp,
				)
			}
		case rawbus.TypeTicker:
			tickers, ok := tickerUpdates(message.Value)

			if !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid ticker",
				))

				continue
			}

			for _, ticker := range tickers {
				if ticker == nil {
					continue
				}

				signal := system.LoadSignal(logic.EntityTick, ticker.Symbol)

				if signal == nil {
					errnie.Error(errors.New(
						string(system.source) + ": symbol not found",
					))

					continue
				}

				system.publishMeasurement(
					signal,
					signal.Record(ticker),
					ticker.Timestamp,
				)
			}
		case rawbus.TypeBook:
			books, ok := bookUpdates(message.Value)

			if !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid book",
				))

				continue
			}

			for _, book := range books {
				if book == nil {
					continue
				}

				signal := system.LoadSignal(logic.EntityBook, book.Symbol)

				if signal == nil {
					errnie.Error(errors.New(
						string(system.source) + ": symbol not found",
					))

					continue
				}

				eventAt := book.Timestamp

				system.publishMeasurement(
					signal,
					signal.Record(book),
					eventAt,
				)
			}
		case rawbus.TypeFeedback:
			feedback, ok := message.Value.(*market.Feedback)

			if !ok {
				errnie.Error(errors.New(
					string(system.source) + ": invalid feedback",
				))

				continue
			}

			system.feedback = feedback
		default:
			continue
		}
	}
}

func (system *System) publishMeasurement(
	signal market.Signal,
	warmed bool,
	eventAt time.Time,
) {
	if signal == nil {
		return
	}

	system.gauge.RecordWarmup(signal.Symbol(), warmed)

	measurement, err := signal.Measure(system.feedback, eventAt)

	if errnie.Error(err) != nil {
		return
	}

	if err := measurement.Publish(system.bus); errnie.Error(err) != nil {
		return
	}

	errnie.Error(system.gauge.Publish(
		measurement,
		signal.Symbol(),
	))
}

func tradeUpdates(value any) (krakenmarket.TradeUpdates, bool) {
	switch updates := value.(type) {
	case krakenmarket.TradeUpdates:
		return updates, true
	case *krakenmarket.TradeUpdates:
		if updates == nil {
			return nil, false
		}

		return *updates, true
	default:
		return nil, false
	}
}

func tickerUpdates(value any) (krakenmarket.TickerUpdates, bool) {
	switch updates := value.(type) {
	case krakenmarket.TickerUpdates:
		return updates, true
	case *krakenmarket.TickerUpdates:
		if updates == nil {
			return nil, false
		}

		return *updates, true
	default:
		return nil, false
	}
}

func bookUpdates(value any) (krakenmarket.BookUpdates, bool) {
	switch updates := value.(type) {
	case krakenmarket.BookUpdates:
		return updates, true
	case *krakenmarket.BookUpdates:
		if updates == nil {
			return nil, false
		}

		return *updates, true
	default:
		return nil, false
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
