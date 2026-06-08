package correlation

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
	"github.com/theapemachine/symm/numeric"
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
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	measurementsCapacity := viper.GetInt("signals.correlation.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

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
		crossSection: newCrossSection(
			minReturnBars(measurementsCapacity),
			measurementsCapacity,
		),
	}
}

func minReturnBars(measurementsCapacity int) int {
	window := measurementsCapacity / 8

	if window < 4 {
		return 4
	}

	return window
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
				errnie.Error(errors.New("correlation: invalid trade"), "correlation: invalid trade")
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("correlation: symbol not found"), "correlation: symbol not found")
				continue
			}

			signal.measurements.Value = trade
			signal.measurements = signal.measurements.Next()
		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("correlation: invalid ticker"), "correlation: invalid ticker")
				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("correlation: symbol not found"), "correlation: symbol not found")
				continue
			}

			signal.measurements.Value = ticker
			signal.measurements = signal.measurements.Next()
		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("correlation: invalid book"), "correlation: invalid book")
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("correlation: symbol not found"), "correlation: symbol not found")
				continue
			}

			signal.measurements.Value = book
			signal.measurements = signal.measurements.Next()
		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("correlation: invalid feedback"), "correlation: invalid feedback")
				continue
			}

			system.feedback = feedback
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

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.correlation.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.correlation.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.correlation.measurements_capacity")

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
		errnie.Error(errors.New("correlation: symbol is not a Signal"), "correlation: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}

type crossSection struct {
	universe sync.Map
	minBars  int
	capacity int
}

type symbolState struct {
	lastPrice float64
	returns   []float64
}

func newCrossSection(minBars, capacity int) *crossSection {
	return &crossSection{
		minBars:  minBars,
		capacity: capacity,
	}
}

func (crossSection *crossSection) publishPrice(symbol string, price float64) {
	if price <= 0 {
		return
	}

	raw, _ := crossSection.universe.LoadOrStore(symbol, &symbolState{})
	state, ok := raw.(*symbolState)

	if !ok {
		return
	}

	if state.lastPrice <= 0 {
		state.lastPrice = price
		return
	}

	if price == state.lastPrice {
		return
	}

	logReturn := math.Log(price / state.lastPrice)
	state.lastPrice = price
	state.returns = append(state.returns, logReturn)

	if len(state.returns) > crossSection.capacity {
		state.returns = state.returns[len(state.returns)-crossSection.capacity:]
	}
}

func (crossSection *crossSection) symbolReturns(symbol string, window int) []float64 {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return nil
	}

	state, ok := raw.(*symbolState)

	if !ok || len(state.returns) < window {
		return nil
	}

	start := len(state.returns) - window

	return append([]float64(nil), state.returns[start:]...)
}

func (crossSection *crossSection) marketReturns(window int) []float64 {
	series := make([][]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		state, ok := value.(*symbolState)

		if !ok || len(state.returns) < window {
			return true
		}

		start := len(state.returns) - window
		series = append(series, state.returns[start:])

		return true
	})

	if len(series) < 2 {
		return nil
	}

	market := make([]float64, window)

	for index := range window {
		values := make([]float64, 0, len(series))

		for _, returns := range series {
			values = append(values, returns[index])
		}

		market[index] = numeric.Median(values)
	}

	return market
}

func (crossSection *crossSection) peerCorrelations(window int) []float64 {
	market := crossSection.marketReturns(window)

	if len(market) < window {
		return nil
	}

	correlations := make([]float64, 0)

	crossSection.universe.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		returns := crossSection.symbolReturns(symbol, window)

		if len(returns) < window {
			return true
		}

		correlations = append(correlations, numeric.Pearson(returns, market))

		return true
	})

	return correlations
}

func (crossSection *crossSection) peerEnergies(window int) []float64 {
	energies := make([]float64, 0)

	crossSection.universe.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		returns := crossSection.symbolReturns(symbol, window)

		if len(returns) < window {
			return true
		}

		energies = append(energies, numeric.MedianAbsolute(returns))

		return true
	})

	return energies
}

func (crossSection *crossSection) minBarsRequired() int {
	return crossSection.minBars
}
