package toxicity

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
	err      error
	pool     *qpool.Q[any]
	bus      *internal.Bus
	signals  sync.Map
	gauge    *telemetry.Gauge
	feedback *market.Feedback
	tracker  *Tracker
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	measurementsCapacity := viper.GetInt("signals.toxicity.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceToxicity)

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
		gauge:   gauge,
		tracker: Default(),
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
				errnie.Error(errors.New("toxicity: invalid trade"), "toxicity: invalid trade")
				continue
			}

			if errnie.Error(system.feedTrade(trade)) != nil {
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("toxicity: symbol not found"), "toxicity: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("toxicity: invalid ticker"), "toxicity: invalid ticker")
				continue
			}

			system.feedTicker(ticker)

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("toxicity: symbol not found"), "toxicity: symbol not found")
				continue
			}

			warmed = signal.Record(ticker)

		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("toxicity: invalid book"), "toxicity: invalid book")
				continue
			}

			if errnie.Error(system.feedBook(book)) != nil {
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("toxicity: symbol not found"), "toxicity: symbol not found")
				continue
			}

			warmed = signal.Record(book)

		case "level3":
			var update *krakenmarket.Level3Update

			switch value := message.Value.(type) {
			case *krakenmarket.Level3Update:
				update = value
			case krakenmarket.Level3Update:
				update = &value
			default:
				errnie.Error(errors.New("toxicity: invalid level3"), "toxicity: invalid level3")
				continue
			}

			if errnie.Error(system.feedLevel3(update)) != nil {
				continue
			}

			if len(update.Bids) == 0 && len(update.Asks) == 0 {
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, update.Symbol)

			if signal == nil {
				errnie.Error(errors.New("toxicity: symbol not found"), "toxicity: symbol not found")
				continue
			}

			warmed = signal.Record(update)

		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("toxicity: invalid feedback"), "toxicity: invalid feedback")
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

func (system *System) feedTrade(trade *krakenmarket.TradeUpdate) error {
	eventAt, err := krakenmarket.EventTimeFromTrade(trade)

	if err != nil {
		return err
	}

	system.tracker.ObserveTrade(
		trade.Symbol,
		krakenmarket.Pair{},
		trade.Price,
		trade.Qty,
		eventAt,
	)

	return nil
}

func (system *System) feedTicker(ticker *krakenmarket.TickerUpdate) {
	pair := krakenmarket.Pair{}

	if ticker.Bid > 0 && ticker.Ask > 0 {
		system.tracker.ObserveMid(ticker.Symbol, pair, (ticker.Bid+ticker.Ask)/2)
	}

	if ticker.Last > 0 {
		system.tracker.ObserveLast(ticker.Symbol, pair, ticker.Last)
	}
}

func (system *System) feedBook(book *krakenmarket.Book) error {
	eventAt, err := krakenmarket.EventTimeFromBook(book)

	if err != nil {
		return err
	}

	system.tracker.ApplyBookFrame(book.Symbol, krakenmarket.Pair{}, book, eventAt)

	return nil
}

func (system *System) feedLevel3(update *krakenmarket.Level3Update) error {
	pair := krakenmarket.Pair{}

	for _, bid := range update.Bids {
		event := bid.Event

		if event == "" {
			event = "add"
		}

		if bid.Timestamp.IsZero() {
			return fmt.Errorf("toxicity: level3 bid %q timestamp is zero", bid.OrderID)
		}

		system.tracker.ApplyOrder(
			update.Symbol,
			pair,
			event,
			bid.OrderID,
			SideBid,
			bid.LimitPrice,
			bid.OrderQty,
			bid.Timestamp,
			bid.Timestamp,
		)
	}

	for _, ask := range update.Asks {
		event := ask.Event

		if event == "" {
			event = "add"
		}

		if ask.Timestamp.IsZero() {
			return fmt.Errorf("toxicity: level3 ask %q timestamp is zero", ask.OrderID)
		}

		system.tracker.ApplyOrder(
			update.Symbol,
			pair,
			event,
			ask.OrderID,
			SideAsk,
			ask.LimitPrice,
			ask.OrderQty,
			ask.Timestamp,
			ask.Timestamp,
		)
	}

	return nil
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.toxicity.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.toxicity.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.toxicity.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	mapKey := fmt.Sprintf("%d:%s", entity, symbol)

	raw, _ = system.signals.LoadOrStore(
		mapKey, NewSignal(
			symbol,
			logic.NewEntity(entity),
			measurementsCapacity,
			system.tracker,
			threshold,
			alpha,
		),
	)

	if signal, ok = raw.(*Signal); !ok {
		errnie.Error(errors.New("toxicity: symbol is not a Signal"), "toxicity: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}
