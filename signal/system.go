package signal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/rawbus"
	"github.com/theapemachine/symm/telemetry"
)

type System struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	pool                *qpool.Q[any]
	bus                 *internal.Bus
	gauge               *telemetry.Gauge
	signals             sync.Map
	knownSymbols        map[string]struct{}
	feedback            *market.Feedback
	source              logic.SourceType
	signal              func(string, *logic.Entity) market.Signal
	onSymbols           func([]string)
	bookHandler         func(*krakenmarket.BookUpdate) (handled bool, err error)
	entities            map[logic.EntityType]struct{}
	acceptedEntityTypes []logic.EntityType
	recorder            *audit.Recorder
	scoreInertia        sync.Map
	scoreInertiaEffort  int
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

	var recorder *audit.Recorder

	if viper.GetBool("system.audit.enabled") {
		recorder, err = audit.NewRecorder(viper.GetString("system.audit.file"))

		if errnie.Error(err) != nil {
			cancel()
			return nil
		}
	}

	return &System{
		ctx:                 ctx,
		cancel:              cancel,
		pool:                pool,
		bus:                 bus,
		gauge:               gauge,
		source:              source,
		signal:              signal,
		entities:            entitySet,
		acceptedEntityTypes: buildAcceptedEntityTypes(entitySet),
		knownSymbols:        make(map[string]struct{}),
		recorder:            recorder,
		scoreInertiaEffort:  resolveScoreInertiaEffort(),
	}
}

func (system *System) Tick() error {
	errnie.Info("signal: starting tick", "source", system.source)

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

			for _, symbol := range symbols {
				system.registerSymbol(symbol)
			}

			if system.onSymbols != nil {
				system.onSymbols(symbols)
			}

			if err := system.publishKnownSymbols(time.Now(), logic.EntityNone, nil); err != nil {
				return errnie.Error(err)
			}
		case rawbus.TypeTrade:
			if !system.accepts(logic.EntityTrade) {
				continue
			}

			trades, ok := message.Value.(*krakenmarket.TradeUpdates)

			if !ok || trades == nil {
				return errnie.Error(errors.New("signal: trades is not a *krakenmarket.TradeUpdates"))
			}

			eventAt, warmed, err := system.ingestTrades(trades)

			if errnie.Error(err) != nil {
				return errnie.Error(err)
			}

			if err := system.publishKnownSymbols(eventAt, logic.EntityTrade, warmed); err != nil {
				return errnie.Error(err)
			}
		case rawbus.TypeTicker:
			if !system.accepts(logic.EntityTick) {
				continue
			}

			tickers, ok := message.Value.(*krakenmarket.TickerUpdates)

			if !ok || tickers == nil {
				return errnie.Error(errors.New("signal: tickers is not a *krakenmarket.TickerUpdates"))
			}

			eventAt, warmed, err := system.ingestTickers(tickers)

			if errnie.Error(err) != nil {
				return errnie.Error(err)
			}

			if err := system.publishKnownSymbols(eventAt, logic.EntityTick, warmed); err != nil {
				return errnie.Error(err)
			}
		case rawbus.TypeBook:
			if !system.accepts(logic.EntityBook) {
				continue
			}

			books, ok := message.Value.(*krakenmarket.BookUpdates)

			if !ok || books == nil {
				return errnie.Error(errors.New("signal: books is not a *krakenmarket.BookUpdates"))
			}

			eventAt, warmed, err := system.ingestBooks(books)

			if errnie.Error(err) != nil {
				return errnie.Error(err)
			}

			if err := system.publishKnownSymbols(eventAt, logic.EntityBook, warmed); err != nil {
				return errnie.Error(err)
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

func (system *System) ingestTrades(
	trades *krakenmarket.TradeUpdates,
) (time.Time, map[string]market.Signal, error) {
	eventAt := time.Time{}
	warmed := make(map[string]market.Signal, len(*trades))

	for _, trade := range *trades {
		if trade == nil {
			return eventAt, warmed, errnie.Error(errors.New("signal: trade is nil"))
		}

		signalInstance, err := system.LoadSignal(logic.EntityTrade, trade.Symbol)

		if err != nil {
			return eventAt, warmed, errnie.Error(err)
		}

		system.recordWarmup(trade.Symbol, signalInstance.Record(trade))
		system.registerSymbol(trade.Symbol)
		warmed[trade.Symbol] = signalInstance
		eventAt = latestEventAt(eventAt, trade.Timestamp)
	}

	return eventAt, warmed, nil
}

func (system *System) ingestTickers(
	tickers *krakenmarket.TickerUpdates,
) (time.Time, map[string]market.Signal, error) {
	eventAt := time.Time{}
	warmed := make(map[string]market.Signal, len(*tickers))

	for _, ticker := range *tickers {
		if ticker == nil {
			return eventAt, warmed, errnie.Error(errors.New("signal: ticker is nil"))
		}

		signalInstance, err := system.LoadSignal(logic.EntityTick, ticker.Symbol)

		if err != nil {
			return eventAt, warmed, errnie.Error(err)
		}

		system.recordWarmup(ticker.Symbol, signalInstance.Record(ticker))
		system.registerSymbol(ticker.Symbol)
		warmed[ticker.Symbol] = signalInstance
		eventAt = latestEventAt(eventAt, ticker.Timestamp)
	}

	return eventAt, warmed, nil
}

func (system *System) ingestBooks(
	books *krakenmarket.BookUpdates,
) (time.Time, map[string]market.Signal, error) {
	eventAt := time.Time{}
	warmed := make(map[string]market.Signal, len(*books))

	for _, book := range *books {
		if book == nil {
			return eventAt, warmed, errnie.Error(errors.New("signal: book is nil"))
		}

		if system.bookHandler != nil {
			handled, handleErr := system.bookHandler(book)

			if handleErr != nil {
				return eventAt, warmed, errnie.Error(handleErr)
			}

			if handled {
				eventAt = latestEventAt(eventAt, book.Timestamp)

				continue
			}
		}

		signalInstance, err := system.LoadSignal(logic.EntityBook, book.Symbol)

		if err != nil {
			return eventAt, warmed, errnie.Error(err)
		}

		system.recordWarmup(book.Symbol, signalInstance.Record(book))
		system.registerSymbol(book.Symbol)
		warmed[book.Symbol] = signalInstance
		eventAt = latestEventAt(eventAt, book.Timestamp)
	}

	return eventAt, warmed, nil
}

/*
publishKnownSymbols measures every known symbol for each accepted entity type.

This is an intentional O(symbols×entities) sweep so cross-symbol slots stay
current after any market event, not only the symbols touched in that event.
*/
func (system *System) publishKnownSymbols(
	eventAt time.Time,
	warmedEntity logic.EntityType,
	warmed map[string]market.Signal,
) error {
	observedAt := eventAt

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	for _, entityType := range system.acceptedEntityTypes {
		useWarmed := entityType == warmedEntity && warmed != nil

		for symbol := range system.knownSymbols {
			signalInstance, loadErr := system.signalForPublish(
				entityType,
				symbol,
				useWarmed,
				warmed,
			)

			if errnie.Error(loadErr) != nil {
				return loadErr
			}

			if measureErr := system.publishMeasurement(signalInstance, observedAt); measureErr != nil {
				return measureErr
			}
		}
	}

	return nil
}

func (system *System) signalForPublish(
	entityType logic.EntityType,
	symbol string,
	useWarmed bool,
	warmed map[string]market.Signal,
) (market.Signal, error) {
	if useWarmed {
		if signalInstance, ok := warmed[symbol]; ok {
			return signalInstance, nil
		}
	}

	return system.LoadSignal(entityType, symbol)
}

func (system *System) publishMeasurement(
	signalInstance market.Signal,
	eventAt time.Time,
) (err error) {
	if signalInstance == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"signal.system.publishMeasurement: unable to publish measurement",
			errors.New("signal is nil"),
		))
	}

	var measurement logic.Measurement

	if measurement, err = signalInstance.Measure(system.feedback, eventAt); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf(
				"signal.system.publishMeasurement: unable to measure signal: %v",
				err,
			),
			err,
		))
	}

	if !measurement.BestEffort && measurement.Symbol != "" {
		state := system.scoreInertiaFor(measurement.Symbol)
		measurement = system.applyScoreInertia(measurement, state)
	}

	if !measurement.Publishable() {
		return nil
	}

	if err := measurement.Publish(system.bus); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"signal.system.publishMeasurement: unable to publish measurement",
			err,
		))
	}

	return nil
}

func (system *System) recordWarmup(symbol string, warmed bool) {
	if system.gauge == nil {
		return
	}

	system.gauge.RecordWarmup(symbol, warmed)
}

func (system *System) registerSymbol(symbol string) {
	if symbol == "" {
		return
	}

	system.knownSymbols[symbol] = struct{}{}
}

func buildAcceptedEntityTypes(entities map[logic.EntityType]struct{}) []logic.EntityType {
	if len(entities) == 0 {
		return []logic.EntityType{
			logic.EntityTrade,
			logic.EntityTick,
			logic.EntityBook,
		}
	}

	accepted := make([]logic.EntityType, 0, len(entities))

	for entityType := range entities {
		accepted = append(accepted, entityType)
	}

	return accepted
}

func latestEventAt(current time.Time, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}

	if current.IsZero() || candidate.After(current) {
		return candidate
	}

	return current
}

func (system *System) LoadSignal(
	entity logic.EntityType, symbol string,
) (market.Signal, error) {
	rawEntities, _ := system.signals.LoadOrStore(symbol, &sync.Map{})
	entitySignals, entityMapOK := rawEntities.(*sync.Map)

	if !entityMapOK {
		return nil, errnie.Error(errors.New("signal: symbol map is not a *sync.Map"))
	}

	raw, _ := entitySignals.LoadOrStore(
		entity,
		system.signal(
			symbol,
			logic.NewEntity(entity),
		),
	)

	signalInstance, ok := raw.(market.Signal)

	if !ok {
		return nil, errnie.Error(errors.New("signal: symbol is not a Signal"))
	}

	return signalInstance, nil
}

func (system *System) Bus() *internal.Bus {
	return system.bus
}

func (system *System) OnSymbols(onSymbols func([]string)) {
	system.onSymbols = onSymbols
}

func (system *System) OnBook(
	handler func(*krakenmarket.BookUpdate) (handled bool, err error),
) {
	system.bookHandler = handler
}

func (system *System) Close() error {
	system.cancel()

	var recorderErr error

	if system.recorder != nil {
		recorderErr = system.recorder.Close()
	}

	return errors.Join(recorderErr, system.bus.Close())
}

func (system *System) accepts(entity logic.EntityType) bool {
	if len(system.entities) == 0 {
		return true
	}

	_, ok := system.entities[entity]

	return ok
}
