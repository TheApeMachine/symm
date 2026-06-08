package leadlag

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

const (
	priceHistoryCap     = 256
	minLagSamples       = 16
	maxLagBars          = 12
	anchorMoveMinObs    = 12
	anchorMoveAlpha     = 0.05
	anchorMoveMinLogRet = 1e-5
	barInterval         = 5 * time.Minute
	ringSampleSpacing   = 15 * time.Second
)

type System struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	bus          *internal.Bus
	signals      sync.Map
	feedback     *market.Feedback
	crossSection *crossSection
	lastPublish  time.Time
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
		signals:      sync.Map{},
		crossSection: newCrossSection(),
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
				errnie.Error(errors.New("leadlag: invalid trade"), "leadlag: invalid trade")
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("leadlag: symbol not found"), "leadlag: symbol not found")
				continue
			}

			signal.measurements.Value = trade
			signal.measurements = signal.measurements.Next()
		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("leadlag: invalid ticker"), "leadlag: invalid ticker")
				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("leadlag: symbol not found"), "leadlag: symbol not found")
				continue
			}

			signal.measurements.Value = ticker
			signal.measurements = signal.measurements.Next()
		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("leadlag: invalid book"), "leadlag: invalid book")
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("leadlag: symbol not found"), "leadlag: symbol not found")
				continue
			}

			signal.measurements.Value = book
			signal.measurements = signal.measurements.Next()
		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("leadlag: invalid feedback"), "leadlag: invalid feedback")
				continue
			}

			system.feedback = feedback
			continue
		}

		measurement, measureErr := signal.Measure(system.feedback)

		if errnie.Error(measureErr) != nil {
			continue
		}

		if measurement.Category == logic.CategoryTypeNone {
			continue
		}

		if !system.throttle() {
			continue
		}

		system.bus.Send(
			"measurements",
			"measurements",
			measurement,
		)
	}
}

func (system *System) throttle() bool {
	interval := viper.GetDuration("signals.leadlag.publish_interval")

	if interval <= 0 {
		interval = 200 * time.Millisecond
	}

	now := time.Now()

	if now.Sub(system.lastPublish) < interval {
		return false
	}

	system.lastPublish = now

	return true
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.leadlag.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.leadlag.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.leadlag.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	mapKey := fmt.Sprintf("%d:%s", entity, symbol)

	raw, _ = system.signals.LoadOrStore(
		mapKey, NewSignal(
			symbol,
			logic.NewEntity(entity),
			ring.New(measurementsCapacity),
			system.crossSection,
			threshold,
			alpha,
		),
	)

	if signal, ok = raw.(*Signal); !ok {
		errnie.Error(errors.New("leadlag: symbol is not a Signal"), "leadlag: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}

func anchorSymbol() string {
	symbol := viper.GetString("market.anchor_symbol")

	if symbol == "" {
		return "BTC/EUR"
	}

	return symbol
}

type crossSection struct {
	universe       sync.Map
	anchorBaseline moveBaseline
}

type symbolState struct {
	mu           sync.RWMutex
	last         float64
	lastSampleAt time.Time
	prices       numeric.PriceSampleRing
}

func newCrossSection() *crossSection {
	return &crossSection{anchorBaseline: newMoveBaseline()}
}

func (crossSection *crossSection) ensure(symbol string) *symbolState {
	raw, _ := crossSection.universe.LoadOrStore(symbol, &symbolState{
		prices: numeric.NewPriceSampleRing(priceHistoryCap),
	})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return state
}

func (crossSection *crossSection) observePrice(symbol string, price float64, at time.Time) {
	if symbol == "" || price <= 0 || at.IsZero() {
		return
	}

	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.last = price

	if !state.lastSampleAt.IsZero() && at.Sub(state.lastSampleAt) < ringSampleSpacing {
		return
	}

	state.lastSampleAt = at
	state.prices.Push(at, price)
}

func (crossSection *crossSection) anchorState() *symbolState {
	return crossSection.ensure(anchorSymbol())
}

func (state *symbolState) priceSamplesInto(destination []numeric.PriceSample) []numeric.PriceSample {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.prices.AppendOrdered(destination)
}

func (state *symbolState) lastPrice() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.last
}

func (state *symbolState) observeTicker(last float64, at time.Time) {
	if last <= 0 || at.IsZero() {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.last = last

	if !state.lastSampleAt.IsZero() && at.Sub(state.lastSampleAt) < ringSampleSpacing {
		return
	}

	state.lastSampleAt = at
	state.prices.Push(at, last)
}
