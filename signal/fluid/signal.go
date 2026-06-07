package fluid

import (
	"context"
	"fmt"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

/*
Signal applies order-book fluid dynamics per symbol and maps the field onto the
mechanical perspective (Laminar / Turbulent / Inertial / Viscous). It consumes
book, trades, and ticks; the field model lives in FluidSymbol.
*/
const rawSubscriberID = "signal/fluid:raw"

type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols       sync.Map
	fieldScratch  []*FluidSymbol
	ui            *qpool.BroadcastGroup
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	categories    map[string]types.CategoryType
	rawDump       *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		fluidDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"laminar", "inertial", "viscous", "turbulent"},
		[]float64{0.40, 0.30, 0.20, 0.10},
		numeric.DefaultCalibratorConfig("strength"),
		"fluid",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryLaminar,
		types.CategoryInertial,
		types.CategoryViscous,
		types.CategoryTurbulent,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/fluid")
		return nil
	}

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscribers:   make(map[string]*qpool.BroadcastConsumer),
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		categories: map[string]types.CategoryType{
			"laminar":   types.CategoryLaminar,
			"inertial":  types.CategoryInertial,
			"viscous":   types.CategoryViscous,
			"turbulent": types.CategoryTurbulent,
		},
		rawDump: rawdump.Open("fluid"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = bus.Group(
			pool, channel, viper.GetDuration("system.queue.ttl"),
		)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(
			rawSubscriberID, viper.GetInt("system.queue.buffer"),
		)
	}

	signal.broadcasts["measurements"] = bus.Group(
		pool, "measurements", viper.GetDuration("system.queue.ttl"),
	)
	signal.ui = bus.Group(pool, "ui", viper.GetDuration("system.queue.ttl"))

	errnie.Info("signal/fluid ready", "signal/fluid")

	return signal
}

func (signal *Signal) state(symbol string) (*FluidSymbol, error) {
	if stored, ok := signal.symbols.Load(symbol); ok {
		return stored.(*FluidSymbol), nil
	}

	created, err := NewFluidSymbol(symbol, signal.classifier)

	if err != nil {
		return nil, err
	}

	stored, _ := signal.symbols.LoadOrStore(symbol, created)

	return stored.(*FluidSymbol), nil
}

func (signal *Signal) Tick() (err error) {
	for {
		message, err := signal.subscribers["raw"].Wait(signal.ctx)

		if err != nil {
			return err
		}

		if message == nil || message.Value == nil {
			continue
		}

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
			continue
		}

		var state *FluidSymbol

		switch sm.Channel {
		case public.TradesChannel:
			for _, trade := range signalpool.GetTrades(sm) {
				if state, err = signal.state(trade.Symbol); errnie.Error(err) != nil {
					continue
				}

				if err = state.FeedTradeSide(
					trade.Timestamp, trade.Qty, trade.Side,
				); errnie.Error(err) != nil {
					continue
				}

				if err = signal.emit(trade.Symbol); errnie.Error(err) != nil {
					continue
				}
			}
		case public.TickerChannel:
			for _, ticker := range signalpool.GetTickers(sm) {
				if state, err = signal.state(ticker.Symbol); errnie.Error(err) != nil {
					continue
				}

				if err = state.FeedTicker(ticker); errnie.Error(err) != nil {
					continue
				}

				if err = signal.emit(ticker.Symbol); errnie.Error(err) != nil {
					continue
				}
			}
		case public.BookChannel:
			for _, delta := range signalpool.GetBooks(sm) {
				if state, err = signal.state(delta.Symbol); errnie.Error(err) != nil {
					continue
				}

				if err = state.FeedBook(delta); errnie.Error(err) != nil {
					continue
				}

				if err = signal.emit(delta.Symbol); errnie.Error(err) != nil {
					continue
				}
			}
		}
	}
}

func (signal *Signal) emit(symbol string) error {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return nil
	}

	state := raw.(*FluidSymbol)
	measurement, standout, err := state.Measure(signal.categories)

	if err != nil {
		return err
	}

	if measurement.Source != types.SourceNone {
		signal.calibrator.Observe(measurement.Strength, signal.classifier)

		telemetry := signal.calibrator.Snapshot(signal.classifier)
		telemetry.Observation = measurement.Strength
		if err := types.AssignCategorySurpriseSNR(
			&measurement, signal.surpriseField, measurement.Category,
		); err != nil {
			return err
		}

		if err := signal.rawDump.Write(rawRecord{
			TimestampUnixNano: time.Now().UTC().UnixNano(),
			Symbol:            measurement.Symbol,
			Category:          measurement.Category,
			Strength:          measurement.Strength,
			Confidence:        measurement.Confidence,
			SNR:               measurement.SNR,
			Standout:          standout,
			Last:              measurement.Last,
			SpreadBPS:         measurement.SpreadBPS,
		}); err != nil {
			return err
		}

		if err := measurement.Send(signal.pool); err != nil {
			return err
		}

		if signal.ui != nil {
			signal.ui.Send(&qpool.QValue[any]{
				Value: numeric.GaugePayload(
					measurement.Source.String(),
					measurement.Symbol,
					measurement.Category,
					measurement,
					telemetry,
				),
			})
		}
	}

	if err := signal.publishField(state); err != nil {
		return err
	}

	return nil
}

// publishField ships an aggregated universe field snapshot to the dashboard
// surface when at least one symbol has field data.
func (signal *Signal) publishField(_ *FluidSymbol) error {
	states := signal.fieldScratch[:0]

	signal.symbols.Range(func(_, value any) bool {
		states = append(states, value.(*FluidSymbol))

		return true
	})

	signal.fieldScratch = states

	tasks := make([]*qpool.ResultWait[any], 0, len(states))

	for _, fluidState := range states {
		tasks = append(tasks, signal.pool.Schedule(fmt.Sprintf("%s:parallel:%p", "fluid", &tasks), func(ctx context.Context) (any, error) {
			row := fluidState.Row()

			if row == nil {
				return nil, nil
			}

			return row, nil
		}))
	}

	rows := make([]map[string]any, 0, len(tasks))
	var err error

	for _, task := range tasks {
		value, getErr := task.Get(signal.ctx)

		if getErr != nil {
			err = errors.Join(err, getErr)
			continue
		}

		if value.Error != nil {
			err = errors.Join(err, value.Error)
			continue
		}

		if value.Value == nil {
			continue
		}

		row, ok := value.Value.(map[string]any)

		if !ok {
			return errors.Join(err, errors.New("fluid: field row has unexpected type"))
		}

		rows = append(rows, row)
	}

	if err != nil {
		return err
	}

	if len(rows) == 0 {
		return nil
	}

	sort.Slice(rows, func(i, j int) bool {
		left, _ := rows[i]["symbol"].(string)
		right, _ := rows[j]["symbol"].(string)

		return left < right
	})

	signal.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"type":         "fluid",
		"ts":           time.Now().UTC().Format(time.RFC3339Nano),
		"symbol_count": len(rows),
		"symbols":      rows,
	}})

	return nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
