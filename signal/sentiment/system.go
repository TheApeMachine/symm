package sentiment

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

	measurementsCapacity := viper.GetInt("signals.sentiment.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	breadthHistoryCapacity := viper.GetInt("signals.sentiment.breadth_history_capacity")

	if breadthHistoryCapacity <= 0 {
		breadthHistoryCapacity = measurementsCapacity
	}

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceSentiment)

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
		crossSection: &crossSection{
			breadthHistory: ring.New(breadthHistoryCapacity),
		},
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
			symbols, symbolOk := message.Value.([]string); if symbolOk { system.gauge.RegisterSymbols(symbols) }
			continue
		case "trades":
			var (
				trade *krakenmarket.TradeUpdate
			)

			if trade, ok = message.Value.(*krakenmarket.TradeUpdate); !ok {
				errnie.Error(errors.New("sentiment: invalid trade"), "sentiment: invalid trade")
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("sentiment: symbol not found"), "sentiment: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

		case "ticker":
			var (
				ticker *krakenmarket.TickerUpdate
			)

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("sentiment: invalid ticker"), "sentiment: invalid ticker")
				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("sentiment: symbol not found"), "sentiment: symbol not found")
				continue
			}

			warmed = signal.Record(ticker)

		case "book":
			var (
				book *krakenmarket.Book
			)

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("sentiment: invalid book"), "sentiment: invalid book")
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("sentiment: symbol not found"), "sentiment: symbol not found")
				continue
			}

			warmed = signal.Record(book)

		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("sentiment: invalid feedback"), "sentiment: invalid feedback")
				continue
			}

			system.feedback = feedback
			continue
		}

		if signal == nil {
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

	threshold := math.Min(math.Max(viper.GetFloat64("signals.sentiment.surge_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.sentiment.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.sentiment.measurements_capacity")

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
		errnie.Error(errors.New("sentiment: symbol is not a Signal"), "sentiment: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}

type crossSection struct {
	universe       sync.Map
	breadthHistory *ring.Ring
}

func (crossSection *crossSection) publishChange(symbol string, change float64) {
	crossSection.universe.Store(symbol, change)
}

func (crossSection *crossSection) snapshot() []float64 {
	changes := make([]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		change, ok := value.(float64)

		if !ok {
			return true
		}

		changes = append(changes, change)

		return true
	})

	return changes
}

func (crossSection *crossSection) breadth() float64 {
	changes := crossSection.snapshot()

	if len(changes) == 0 {
		return 0
	}

	positive := 0

	for _, change := range changes {
		if change > 0 {
			positive++
		}
	}

	return float64(positive) / float64(len(changes))
}

func (crossSection *crossSection) recordBreadth(breadth float64) {
	crossSection.breadthHistory.Value = breadth
	crossSection.breadthHistory = crossSection.breadthHistory.Next()
}

func (crossSection *crossSection) majorityThreshold() float64 {
	changes := crossSection.snapshot()
	universeSize := len(changes)

	if universeSize <= 0 {
		return 1
	}

	required := universeSize/2 + 1

	return float64(required) / float64(universeSize)
}

func (crossSection *crossSection) isLeader(symbolChange float64) bool {
	changes := crossSection.snapshot()

	if symbolChange <= 0 || len(changes) == 0 {
		return false
	}

	return symbolChange >= numeric.Max(changes)
}
