package liquidity

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

	measurementsCapacity := viper.GetInt("signals.liquidity.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceLiquidity)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	return &System{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		bus:          bus,
		signals:      sync.Map{},
		gauge:        gauge,
		crossSection: &crossSection{},
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
				errnie.Error(errors.New("liquidity: invalid trade"), "liquidity: invalid trade")
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("liquidity: symbol not found"), "liquidity: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

		case "ticker":
			var (
				ticker *krakenmarket.TickerUpdate
			)

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("liquidity: invalid ticker"), "liquidity: invalid ticker")
				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("liquidity: symbol not found"), "liquidity: symbol not found")
				continue
			}

			warmed = signal.Record(ticker)

		case "book":
			var (
				book *krakenmarket.Book
			)

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("liquidity: invalid book"), "liquidity: invalid book")
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("liquidity: symbol not found"), "liquidity: symbol not found")
				continue
			}

			warmed = signal.Record(book)

		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("liquidity: invalid feedback"), "liquidity: invalid feedback")
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

	threshold := math.Min(math.Max(viper.GetFloat64("signals.liquidity.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.liquidity.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.liquidity.measurements_capacity")

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
		errnie.Error(errors.New("liquidity: symbol is not a Signal"), "liquidity: symbol is not a Signal")
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
}

func (crossSection *crossSection) publishQuoteVol(symbol string, quoteVol float64) {
	crossSection.universe.Store(symbol, quoteVol)
}

func (crossSection *crossSection) snapshot() []float64 {
	volumes := make([]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		quoteVol, ok := value.(float64)

		if !ok {
			return true
		}

		volumes = append(volumes, quoteVol)

		return true
	})

	return volumes
}
