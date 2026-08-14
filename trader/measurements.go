package trader

import (
	"context"
	"slices"
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
	quotes := types.NewQuoteHistory(system.Cfg.PumpDump.Capacity)

	return newMeasurements(
		ctx,
		ui,
		[]types.Signal{
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
	)
}

func newMeasurements(
	ctx context.Context,
	ui chan []byte,
	signals []types.Signal,
) *Measurements {
	ctx, cancel := context.WithCancel(ctx)

	return &Measurements{ctx: ctx, cancel: cancel, ui: ui, signals: signals}
}

/*
Update runs selected signals serially for each symbol. Production streaming
uses MeasureSymbol directly; this whole-thesis path remains for focused tests
and callers that deliberately request a synchronous sweep.
*/
func (measurements *Measurements) Update(
	thesis *types.Thesis,
	receivers []types.SourceType,
) (bool, error) {
	if measurements == nil || thesis == nil {
		return false, errnie.Err(
			errnie.Validation,
			"measurements: conditioner and thesis required",
			nil,
		)
	}

	resonanceReady := false

	for _, signal := range measurements.signals {
		if !slices.Contains(receivers, signal.Type()) {
			continue
		}

		var measurementErr error

		thesis.Symbols.Range(func(_, value any) bool {
			symbol, ok := value.(*types.Symbol)

			if !ok {
				measurementErr = errnie.Err(
					errnie.Internal,
					"measurements: symbol type assertion failed",
					nil,
				)
				return false
			}

			symbol.Tick = thesis.Tick
			rows, err := measurements.measure(
				symbol,
				thesis.Tick,
				[]types.Signal{signal},
			)

			if err != nil {
				measurementErr = err
				return false
			}

			resonanceReady = measurements.commit(thesis, rows) || resonanceReady
			return true
		})

		if measurementErr != nil {
			return false, measurementErr
		}
	}

	return resonanceReady, nil
}

/*
MeasureSymbol advances only the selected signal owners for one market event.
It is the streaming hot path and never scans unrelated symbols.
*/
func (measurements *Measurements) MeasureSymbol(
	symbol *types.Symbol,
	tick int64,
	receivers []types.SourceType,
) ([]*types.Measurement, error) {
	if measurements == nil || symbol == nil {
		return nil, errnie.Err(
			errnie.Validation,
			"measurements: conditioner and symbol required",
			nil,
		)
	}

	selected := make([]types.Signal, 0, len(receivers))

	for _, signal := range measurements.signals {
		if slices.Contains(receivers, signal.Type()) {
			selected = append(selected, signal)
		}
	}

	return measurements.measure(symbol, tick, selected)
}

func (measurements *Measurements) measure(
	symbol *types.Symbol,
	tick int64,
	signals []types.Signal,
) ([]*types.Measurement, error) {
	rows := make([]*types.Measurement, 0)

	for _, signal := range signals {
		started := time.Now()

		if measurements.clocks != nil && !measurements.dispatchedAt.IsZero() {
			measurements.clocks.observeHop(
				"crypto",
				string(signal.Type()),
				started.Sub(measurements.dispatchedAt),
			)
		}

		for _, measurement := range signal.Measure(symbol, tick) {
			if measurement == nil {
				continue
			}

			measurement.Tick = tick

			rows = append(rows, measurement)
		}

		if measurements.clocks != nil {
			measurements.clocks.observe(string(signal.Type()), time.Since(started))
		}
	}

	return rows, nil
}

func (measurements *Measurements) commit(
	thesis *types.Thesis,
	rows []*types.Measurement,
) bool {
	resonanceReady := false
	focused := make([]*types.Measurement, 0)

	for _, measurement := range rows {
		if thesis.Symbol(measurement.Symbol).AppendMeasurement(
			measurement.Source,
			measurement,
		) {
			resonanceReady = true
		}

		if measurement.Symbol == types.Focus() {
			focused = append(focused, measurement)
		}
	}

	if len(focused) > 0 {
		utils.Publish(measurements.ui, datura.NewMap(
			"measurements", focused,
		))
	}

	return resonanceReady
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
