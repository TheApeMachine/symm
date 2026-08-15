package trader

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

type Measurements struct {
	ctx          context.Context
	cancel       context.CancelFunc
	signals      []types.Signal
	ui           chan []byte
	clocks       *clockBank
	dispatchedAt time.Time
}

func NewMeasurements(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	ui chan []byte,
) *Measurements {
	ctx, cancel := context.WithCancel(ctx)

	quotes := types.NewQuoteHistory(system.Cfg.PumpDump.Capacity)

	return &Measurements{
		ctx:    ctx,
		cancel: cancel,
		ui:     ui,
		signals: []types.Signal{
			correlation.NewSignal(ctx, api, ui),
			cvd.NewSignalWithQuotes(ctx, api, ui, quotes),
			depthflow.NewSignal(ctx, api, instrument, ui),
			exhaust.NewSignal(ctx, api, instrument, ui),
			hawkes.NewSignal(ctx, api, ui),
			leadlag.NewSignal(ctx, api, ui),
			liquidity.NewSignal(ctx, api, ui),
			pumpdump.NewSignalWithQuotes(ctx, api, ui, quotes),
			sentiment.NewSignal(ctx, api, ui),
			toxicity.NewSignal(ctx, api, ui),
		},
	}
}

/*
Generate calls the signals on a per-symbol basis, and runs a map-reduce to collect
the measurements into the thesis. It returns true if any symbol is ready for
resonance, and an error if any signal failed to measure.
*/
func (measurements *Measurements) Generate(
	thesis *types.Thesis, receivers []types.SourceType,
) (bool, error) {
	var ready atomic.Bool
	group, _ := errgroup.WithContext(measurements.ctx)

	thesis.Tick++
	utils.PublishPriority(measurements.ui, datura.NewMap(
		"tick", datura.NewMap("count", thesis.Tick),
	))

	for _, signal := range measurements.signals {
		if !slices.Contains(receivers, signal.Type()) {
			continue
		}

		activeSignal := signal
		group.Go(func() error {
			utils.PublishPriority(measurements.ui, datura.NewMap("activity", datura.NewMap(
				string(activeSignal.Type()), "running",
			)))

			defer utils.PublishPriority(measurements.ui, datura.NewMap("activity", datura.NewMap(
				string(activeSignal.Type()), "done",
			)))

			var signalMeasurements []*types.Measurement

			thesis.Symbols.Range(func(_, value any) bool {
				symbol, ok := value.(*types.Symbol)

				if !ok {
					errnie.Err(
						errnie.Internal,
						"measurements: symbol type assertion failed",
						nil,
					)

					return false
				}

				symbol.Tick = thesis.Tick
				started := time.Now()
				measured := activeSignal.Measure(symbol, thesis.Tick)

				if len(measured) == 0 {
					return true
				}

				if measurements.clocks != nil {
					measurements.clocks.observe(activeSignal.Name(), time.Since(started))
				}

				for _, measurement := range measured {
					measurement.Tick = thesis.Tick

					if thesis.Symbol(measurement.Symbol).AppendMeasurement(
						measurement.Source,
						measurement,
					) {
						ready.Store(true)
					}
				}

				signalMeasurements = append(signalMeasurements, measured...)
				return true
			})

			if len(signalMeasurements) > 0 {
				utils.Publish(measurements.ui, datura.NewMap("measurements", signalMeasurements))
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return false, errnie.Error(errnie.Err(
			errnie.Internal,
			"measurements: failed to generate measurements",
			err,
		))
	}

	return ready.Load(), nil
}

/*
Close releases every signal conditioner owned by the measurement stage.
*/
func (measurements *Measurements) Close() error {
	if measurements == nil {
		return nil
	}

	measurements.cancel()

	for _, signal := range measurements.signals {
		if err := signal.Close(); err != nil {
			return err
		}
	}

	return nil
}
