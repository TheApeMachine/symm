package correlation

import (
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
	"github.com/theapemachine/symm/telemetry"
)

type System struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	bus          *internal.Bus
	signals      sync.Map
	gauge        *telemetry.Gauge
	feedback     *market.Feedback
	crossSection *crossSection
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	measurementsCapacity := viper.GetInt("signals.correlation.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceCorrelation)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	crossSection, crossSectionErr := newCrossSection(
		minReturnBars(measurementsCapacity),
		measurementsCapacity,
	)

	if crossSectionErr != nil {
		cancel()
		errnie.Error(crossSectionErr)

		return nil
	}

	return &System{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		bus:          bus,
		signals:      sync.Map{},
		gauge:        gauge,
		crossSection: crossSection,
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
				errnie.Error(errors.New("correlation: invalid trade"), "correlation: invalid trade")
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("correlation: symbol not found"), "correlation: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

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

			warmed = signal.Record(ticker)

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

			warmed = signal.Record(book)

		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("correlation: invalid feedback"), "correlation: invalid feedback")
				continue
			}

			system.feedback = feedback
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
			measurementsCapacity,
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
	universe    sync.Map
	minBars     int
	capacity    int
	matchWindow time.Duration
}

type symbolState struct {
	lastPrice  float64
	lastTickAt time.Time
	returns    []float64
}

func newCrossSection(minBars, capacity int) (*crossSection, error) {
	matchWindow := viper.GetDuration("signals.trade_match_window")

	if matchWindow <= 0 {
		return nil, fmt.Errorf("signals.trade_match_window must be positive")
	}

	return &crossSection{
		minBars:     minBars,
		capacity:    capacity,
		matchWindow: matchWindow,
	}, nil
}

func (crossSection *crossSection) publishPrice(symbol string, price float64, at time.Time) {
	if price <= 0 {
		return
	}

	raw, _ := crossSection.universe.LoadOrStore(symbol, &symbolState{})
	state, ok := raw.(*symbolState)

	if !ok {
		return
	}

	if at.IsZero() {
		return
	}

	state.lastTickAt = at

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

func (crossSection *crossSection) stalenessWeight(updatedAt time.Time, now time.Time) float64 {
	elapsed := now.Sub(updatedAt)

	if elapsed >= crossSection.matchWindow {
		return 0
	}

	return math.Exp(-float64(elapsed) / float64(crossSection.matchWindow))
}

func (crossSection *crossSection) symbolFresh(state *symbolState, now time.Time) bool {
	if state == nil || state.lastTickAt.IsZero() {
		return false
	}

	return crossSection.stalenessWeight(state.lastTickAt, now) > 0
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

func (crossSection *crossSection) marketReturns(window int, at time.Time) []float64 {
	series := make([][]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		state, ok := value.(*symbolState)

		if !ok || len(state.returns) < window || !crossSection.symbolFresh(state, at) {
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

func (crossSection *crossSection) peerCorrelations(window int, at time.Time) []float64 {
	market := crossSection.marketReturns(window, at)

	if len(market) < window {
		return nil
	}

	correlations := make([]float64, 0)

	crossSection.universe.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		raw, loaded := crossSection.universe.Load(symbol)

		if !loaded {
			return true
		}

		state, ok := raw.(*symbolState)

		if !ok || !crossSection.symbolFresh(state, at) {
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

func (crossSection *crossSection) peerEnergies(window int, at time.Time) []float64 {
	energies := make([]float64, 0)

	crossSection.universe.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		raw, loaded := crossSection.universe.Load(symbol)

		if !loaded {
			return true
		}

		state, ok := raw.(*symbolState)

		if !ok || !crossSection.symbolFresh(state, at) {
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

func (crossSection *crossSection) symbolAge(symbol string, now time.Time) time.Duration {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return 0
	}

	state, ok := raw.(*symbolState)

	if !ok || state.lastTickAt.IsZero() {
		return 0
	}

	return now.Sub(state.lastTickAt)
}

func (crossSection *crossSection) minBarsRequired() int {
	return crossSection.minBars
}
