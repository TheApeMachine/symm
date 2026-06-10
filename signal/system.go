package signal

import (
	"context"
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

		if errnie.Error(err) != nil {
			return err
		}

		if message == nil {
			continue
		}

		switch rawbus.TypeFrom(message.Type) {
		case rawbus.TypeSymbols:
			symbols, ok := message.Value.([]string)

			if ok {
				system.gauge.RegisterSymbols(symbols)
			}

			continue
		case rawbus.TypeTrade:
			trades, ok := message.Value.([]*krakenmarket.TradeUpdate)

			if !ok {
				return fmt.Errorf("%s: invalid trade", system.source)
			}

			for _, trade := range trades {
				if trade == nil {
					continue
				}

				signal, loadErr := system.LoadSignal(logic.EntityTrade, trade.Symbol)

				if loadErr != nil {
					return loadErr
				}

				if publishErr := system.publishMeasurement(
					signal,
					signal.Record(trade),
					trade.Timestamp,
				); publishErr != nil {
					return publishErr
				}
			}
		case rawbus.TypeTicker:
			tickers, ok := message.Value.([]*krakenmarket.TickerUpdate)

			if !ok {
				return fmt.Errorf("%s: invalid ticker", system.source)
			}

			for _, ticker := range tickers {
				if ticker == nil {
					continue
				}

				signal, loadErr := system.LoadSignal(logic.EntityTick, ticker.Symbol)

				if loadErr != nil {
					return loadErr
				}

				if publishErr := system.publishMeasurement(
					signal,
					signal.Record(ticker),
					ticker.Timestamp,
				); publishErr != nil {
					return publishErr
				}
			}
		case rawbus.TypeBook:
			books, ok := message.Value.([]*krakenmarket.BookUpdate)

			if !ok {
				return fmt.Errorf("%s: invalid book", system.source)
			}

			for _, book := range books {
				if book == nil {
					continue
				}

				signal, loadErr := system.LoadSignal(logic.EntityBook, book.Symbol)

				if loadErr != nil {
					return loadErr
				}

				if publishErr := system.publishMeasurement(
					signal,
					signal.Record(book),
					book.Timestamp,
				); publishErr != nil {
					return publishErr
				}
			}
		case rawbus.TypeFeedback:
			feedback, ok := message.Value.(*market.Feedback)

			if !ok {
				return fmt.Errorf("%s: invalid feedback", system.source)
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
) error {
	if signal == nil {
		return fmt.Errorf("%s: nil signal", system.source)
	}

	system.gauge.RecordWarmup(signal.Symbol(), warmed)

	measurement, err := signal.Measure(system.feedback, eventAt)

	if err != nil {
		return err
	}

	if err := measurement.Publish(system.bus); err != nil {
		return err
	}

	return system.gauge.Publish(measurement, signal.Symbol())
}

func (system *System) LoadSignal(
	entity logic.EntityType, symbol string,
) (market.Signal, error) {
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

	signal, ok = raw.(market.Signal)

	if !ok {
		return nil, fmt.Errorf("%s: symbol is not a Signal", system.source)
	}

	return signal, nil
}

func (system *System) Bus() *internal.Bus {
	return system.bus
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}
