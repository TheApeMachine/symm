package prediction

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
	chart    *Chart
	feedback *market.Feedback
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw", "measurements"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourcePrediction)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	horizon := viper.GetDuration("story.prediction.horizon")

	return &System{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		bus:     bus,
		signals: sync.Map{},
		gauge:   gauge,
		chart:   NewChart(bus, horizon),
	}
}

func (system *System) Tick() error {
	for {
		for {
			measurementRow, pollErr := system.bus.Poll("measurements")

			if errnie.Error(pollErr) != nil || measurementRow == nil {
				break
			}

			if processErr := system.processMessage(measurementRow); processErr != nil {
				return processErr
			}
		}

		message, err := system.bus.Receive("raw")

		if errnie.Error(err) != nil || message == nil {
			continue
		}

		if processErr := system.processMessage(message); processErr != nil {
			return processErr
		}
	}
}

func (system *System) processMessage(message *qpool.QValue[any]) error {
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

		return nil
	case "measurements":
		var measurement logic.Measurement

		if measurement, ok = message.Value.(logic.Measurement); !ok {
			errnie.Error(errors.New("prediction: invalid measurement"), "prediction: invalid measurement")
			return nil
		}

		if measurement.Symbol == "" || measurement.Source == logic.SourcePrediction {
			return nil
		}

		signal = system.LoadSignal(logic.EntityMeasurement, measurement.Symbol)

		if signal == nil {
			errnie.Error(errors.New("prediction: symbol not found"), "prediction: symbol not found")
			return nil
		}

		signal.Record(measurement)
		signal.rebuildFeaturesFromRing()

		return nil
	case "trades":
		var trade *krakenmarket.TradeUpdate

		if trade, ok = message.Value.(*krakenmarket.TradeUpdate); !ok {
			errnie.Error(errors.New("prediction: invalid trade"), "prediction: invalid trade")
			return nil
		}

		signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

		if signal == nil {
			errnie.Error(errors.New("prediction: symbol not found"), "prediction: symbol not found")
			return nil
		}

		featureSignal := system.LoadSignal(logic.EntityMeasurement, trade.Symbol)

		if featureSignal != nil {
			signal.ApplyFeatures(featureSignal.Features())
		}

		warmed = signal.Record(trade)
	case "ticker":
		var ticker *krakenmarket.TickerUpdate

		if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
			errnie.Error(errors.New("prediction: invalid ticker"), "prediction: invalid ticker")
			return nil
		}

		signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

		if signal == nil {
			errnie.Error(errors.New("prediction: symbol not found"), "prediction: symbol not found")
			return nil
		}

		warmed = signal.Record(ticker)

	case "book":
		var book *krakenmarket.Book

		if book, ok = message.Value.(*krakenmarket.Book); !ok {
			errnie.Error(errors.New("prediction: invalid book"), "prediction: invalid book")
			return nil
		}

		signal = system.LoadSignal(logic.EntityBook, book.Symbol)

		if signal == nil {
			errnie.Error(errors.New("prediction: symbol not found"), "prediction: symbol not found")
			return nil
		}

		warmed = signal.Record(book)

	case "feedback":
		var feedback *market.Feedback

		if feedback, ok = message.Value.(*market.Feedback); !ok {
			errnie.Error(errors.New("prediction: invalid feedback"), "prediction: invalid feedback")
			return nil
		}

		system.feedback = feedback
		return nil
	}

	if signal == nil {
		return nil
	}

	eventAt, eventErr := krakenmarket.EventTimeFromBus(message.Type, message.Value)

	if errnie.Error(eventErr) != nil {
		return nil
	}

	measurement, measureErr := signal.Measure(system.feedback, eventAt)

	if errnie.Error(measureErr) != nil {
		return nil
	}

	if signal.entity.Type == logic.EntityMeasurement {
		return nil
	}

	if publishErr := measurement.Publish(system.bus); errnie.Error(publishErr) != nil {
		return nil
	}

	errnie.Error(system.gauge.Publish(
		measurement,
		signal.symbol,
		warmed,
	))

	if feedback := signal.DrainFeedback(); feedback != nil {
		errnie.Error(system.bus.Send("raw", "feedback", feedback))
	}

	chartEvents := signal.DrainChartEvents()

	if chartEvents.HasForecast || len(chartEvents.Settlements) > 0 {
		errnie.Error(system.chart.Apply(signal.symbol, chartEvents))
	}

	return nil
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	capacity := viper.GetInt("signals.prediction.measurements_capacity")

	if capacity <= 0 {
		errnie.Error(errors.New("prediction: measurements_capacity must be positive"))
		return nil
	}

	horizon := viper.GetDuration("story.prediction.horizon")
	learningRate := math.Min(math.Max(viper.GetFloat64("story.prediction.alpha"), 0.01), 1.0)
	initialVariance := viper.GetFloat64("story.prediction.rls_initial_variance")

	if initialVariance <= 0 {
		errnie.Error(errors.New("story.prediction.rls_initial_variance must be positive"))
		return nil
	}

	mapKey := fmt.Sprintf("%d:%s", entity, symbol)

	if existing, loaded := system.signals.Load(mapKey); loaded {
		if signal, ok = existing.(*Signal); ok {
			return signal
		}
	}

	built, buildErr := NewSignal(
		symbol,
		logic.NewEntity(entity),
		capacity,
		horizon,
		learningRate,
		initialVariance,
	)

	if buildErr != nil {
		errnie.Error(buildErr)
		return nil
	}

	raw, _ = system.signals.LoadOrStore(mapKey, built)

	if signal, ok = raw.(*Signal); !ok {
		errnie.Error(errors.New("prediction: symbol is not a Signal"), "prediction: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}
