package signal

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	ctx       context.Context
	cancel    context.CancelFunc
	pool      *qpool.Q[any]
	bus       *internal.Bus
	signals   sync.Map
	gauge     *telemetry.Gauge
	feedback  *market.Feedback
	source    logic.SourceType
	signal    func(string, *logic.Entity) market.Signal
	onSymbols func([]string)
	entities  map[logic.EntityType]struct{}
}

func NewSystem(
	ctx context.Context,
	pool *qpool.Q[any],
	source logic.SourceType,
	signal func(string, *logic.Entity) market.Signal,
	entities ...logic.EntityType,
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

	entitySet := make(map[logic.EntityType]struct{}, len(entities))

	for _, entity := range entities {
		entitySet[entity] = struct{}{}
	}

	return &System{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		bus:      bus,
		signals:  sync.Map{},
		gauge:    gauge,
		source:   source,
		signal:   signal,
		entities: entitySet,
	}
}

func (system *System) Tick() error {
	for {
		message, err := system.bus.Receive(internal.ChannelRaw)

		if internal.ReportError(err) != nil {
			return err
		}

		if message == nil {
			continue
		}

		switch rawbus.TypeFrom(message.Type) {
		case rawbus.TypeSymbols:
			symbols, ok := message.Value.([]string)

			if !ok {
				return errnie.Error(fmt.Errorf("signal: symbols is not a []string"))
			}

			system.gauge.RegisterSymbols(symbols)

			if system.onSymbols != nil {
				system.onSymbols(symbols)
			}

			continue
		case rawbus.TypeTrade:
			if !system.accepts(logic.EntityTrade) {
				continue
			}

			trades, ok := message.Value.(*krakenmarket.TradeUpdates)

			if !ok || trades == nil {
				return errnie.Error(errors.New("signal: trades is not a *krakenmarket.TradeUpdates"))
			}

			for _, trade := range *trades {
				if trade == nil {
					return errnie.Error(errors.New("signal: trade is nil"))
				}

				signal, err := system.LoadSignal(logic.EntityTrade, trade.Symbol)

				if err != nil {
					return errnie.Error(err)
				}

				if err := system.publishMeasurement(
					signal,
					signal.Record(trade),
					trade.Timestamp,
				); err != nil {
					return errnie.Error(err)
				}
			}
		case rawbus.TypeTicker:
			if !system.accepts(logic.EntityTick) {
				continue
			}

			tickers, ok := message.Value.(*krakenmarket.TickerUpdates)

			if !ok || tickers == nil {
				return errnie.Error(errors.New("signal: tickers is not a *krakenmarket.TickerUpdates"))
			}

			for _, ticker := range *tickers {
				if ticker == nil {
					return errnie.Error(errors.New("signal: ticker is nil"))
				}

				signal, err := system.LoadSignal(logic.EntityTick, ticker.Symbol)

				if err != nil {
					return errnie.Error(err)
				}

				if err := system.publishMeasurement(
					signal,
					signal.Record(ticker),
					ticker.Timestamp,
				); err != nil {
					return errnie.Error(err)
				}
			}
		case rawbus.TypeBook:
			if !system.accepts(logic.EntityBook) {
				continue
			}

			books, ok := message.Value.(*krakenmarket.BookUpdates)

			if !ok || books == nil {
				return errnie.Error(errors.New("signal: books is not a *krakenmarket.BookUpdates"))
			}

			for _, book := range *books {
				if book == nil {
					return errnie.Error(errors.New("signal: book is nil"))
				}

				signal, err := system.LoadSignal(logic.EntityBook, book.Symbol)

				if err != nil {
					return errnie.Error(err)
				}

				if err := system.publishMeasurement(
					signal,
					signal.Record(book),
					book.Timestamp,
				); err != nil {
					return errnie.Error(err)
				}
			}
		case rawbus.TypeFeedback:
			feedback, ok := message.Value.(*market.Feedback)

			if !ok {
				return errnie.Error(errors.New("signal: feedback is not a *market.Feedback"))
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
		return errnie.Error(errors.New("signal: nil signal"))
	}

	system.gauge.RecordWarmup(signal.Symbol(), warmed)

	if warmed {
		return nil
	}

	measurement, err := signal.Measure(system.feedback, eventAt)

	if err != nil {
		if measurementDeferred(err) {
			return nil
		}

		return errnie.Error(err)
	}

	if !measurement.Publishable() {
		return nil
	}

	if err := measurement.Publish(system.bus); err != nil {
		return errnie.Error(err)
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
		return nil, errnie.Error(errors.New("signal: symbol is not a Signal"))
	}

	return signal, nil
}

func (system *System) Bus() *internal.Bus {
	return system.bus
}

func (system *System) OnSymbols(onSymbols func([]string)) {
	system.onSymbols = onSymbols
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}

func (system *System) accepts(entity logic.EntityType) bool {
	if len(system.entities) == 0 {
		return true
	}

	_, ok := system.entities[entity]

	return ok
}

func measurementDeferred(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()

	return strings.Contains(message, ": not ready") ||
		strings.Contains(message, ": insufficient window") ||
		strings.Contains(message, ": insufficient trade window")
}
