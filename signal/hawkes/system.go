package hawkes

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
)

type System struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	pool     *qpool.Q[any]
	bus      *internal.Bus
	signals  sync.Map
	symbols  sync.Map
	feedback *market.Feedback
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	return &System{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"raw"},
			[]string{"measurements"},
		),
		signals: sync.Map{},
		symbols: sync.Map{},
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
				errnie.Error(errors.New("hawkes: invalid trade"), "hawkes: invalid trade")
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("hawkes: symbol not found"), "hawkes: symbol not found")
				continue
			}

			signal.measurements.Value = trade
			signal.measurements = signal.measurements.Next()
		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("hawkes: invalid ticker"), "hawkes: invalid ticker")
				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("hawkes: symbol not found"), "hawkes: symbol not found")
				continue
			}

			signal.measurements.Value = ticker
			signal.measurements = signal.measurements.Next()
		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("hawkes: invalid book"), "hawkes: invalid book")
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("hawkes: symbol not found"), "hawkes: symbol not found")
				continue
			}

			signal.measurements.Value = book
			signal.measurements = signal.measurements.Next()
		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("hawkes: invalid feedback"), "hawkes: invalid feedback")
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
	}
}

func (system *System) loadSymbol(symbol string) *HawkesSymbol {
	raw, _ := system.symbols.LoadOrStore(symbol, NewHawkesSymbol())

	state, ok := raw.(*HawkesSymbol)

	if !ok {
		return nil
	}

	return state
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.hawkes.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.hawkes.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.hawkes.measurements_capacity")

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
		errnie.Error(errors.New("hawkes: symbol is not a Signal"), "hawkes: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}
