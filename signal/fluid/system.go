package fluid

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	disruptor "github.com/smarty/go-disruptor"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
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
	resyncing       sync.Map
	resyncPending   sync.Map
	resyncLastFlush time.Time
	gauge           *telemetry.Gauge
	feedback        *market.Feedback
	ingest          disruptor.Disruptor
	ingestHandler   *ingestHandler
	ingestRing      []ingestEvent
	ingestWaitGroup sync.WaitGroup
	ingestErr       atomic.Value
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui", "kraken:public"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceFluid)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	system := &System{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		bus:     bus,
		signals: sync.Map{},
		symbols: sync.Map{},
		gauge:   gauge,
	}

	if ingestErr := system.startIngest(); ingestErr != nil {
		cancel()
		errnie.Error(ingestErr)

		return nil
	}

	return system
}

func (system *System) Tick() error {
	for {
		system.flushBookResyncs()

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
				errnie.Error(errors.New("fluid: invalid trade"), "fluid: invalid trade")
				continue
			}

			system.feedTrade(trade)

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("fluid: symbol not found"), "fluid: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

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

			warmed = signal.Record(ticker)

		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("fluid: invalid book"), "fluid: invalid book")
				continue
			}

			if feedErr := system.feedBook(book); feedErr != nil {
				errnie.Error(feedErr)
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("fluid: symbol not found"), "fluid: symbol not found")
				continue
			}

			warmed = signal.Record(book)
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

		system.bus.Send(
			"measurements",
			"measurements",
			measurement,
		)

		errnie.Error(system.gauge.Publish(
			measurement,
			signal.symbol,
			warmed,
		))

		errnie.Error(system.publishFieldSnapshot(eventAt))
	}
}

func (system *System) feedTrade(trade *krakenmarket.TradeUpdate) {
	if err := system.publishIngest(ingestEvent{
		symbol:    trade.Symbol,
		kind:      ingestTrade,
		tradeAt:   trade.Timestamp,
		tradeQty:  trade.Qty,
		tradeSide: trade.Side,
	}); err != nil {
		errnie.Error(err)
	}
}

func (system *System) feedTicker(ticker *krakenmarket.TickerUpdate) error {
	tickerAt, tickerErr := krakenmarket.EventTimeFromTicker(ticker)

	if tickerErr != nil {
		return tickerErr
	}

	return system.publishIngest(ingestEvent{
		symbol:   ticker.Symbol,
		kind:     ingestTicker,
		ticker:   *ticker,
		tickerAt: tickerAt,
	})
}

func (system *System) feedBook(book *krakenmarket.Book) error {
	bookAt, bookErr := krakenmarket.EventTimeFromBook(book)

	if bookErr != nil {
		return bookErr
	}

	if err := system.publishIngest(ingestEvent{
		symbol: book.Symbol,
		kind:   ingestBook,
		book:   *book,
		bookAt: bookAt,
	}); err != nil {
		return err
	}

	state := system.loadSymbol(book.Symbol)

	if state == nil {
		return errnie.Error(fmt.Errorf("fluid: symbol %q not found", book.Symbol))
	}

	if !state.Diverged() {
		system.resyncing.Delete(book.Symbol)
		return nil
	}

	if _, pending := system.resyncing.Load(book.Symbol); pending {
		return nil
	}

	system.resyncPending.Store(book.Symbol, struct{}{})

	return nil
}

func (system *System) flushBookResyncs() {
	pace := viper.GetDuration("market.subscribe_pace")

	if time.Since(system.resyncLastFlush) < pace {
		return
	}

	batchSize := viper.GetInt("market.subscribe_batch")
	symbols := make([]string, 0, batchSize)

	system.resyncPending.Range(func(key, value any) bool {
		symbols = append(symbols, key.(string))
		system.resyncPending.Delete(key)

		return len(symbols) < batchSize
	})

	if len(symbols) == 0 {
		return
	}

	system.resyncLastFlush = time.Now()

	for _, symbol := range symbols {
		system.resyncing.Store(symbol, true)
	}

	bookDepth := viper.GetInt("market.book_depth_levels")
	params := krakenmarket.NewBookParams(symbols, bookDepth)
	requestID := time.Now().UnixNano()

	errnie.Error(system.bus.Send("kraken:public", "unsubscribe", types.KrakenMessage{
		Method: "unsubscribe",
		Params: params,
		ReqID:  requestID,
	}))

	errnie.Error(system.bus.Send("kraken:public", "book", types.KrakenMessage{
		Method: "subscribe",
		Params: params,
		ReqID:  requestID + 1,
	}))
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
			measurementsCapacity,
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
	system.closeIngest()

	return system.bus.Close()
}
