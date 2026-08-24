package correlation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	thesis       *types.Thesis
	number       *nomagique.Number[string]
	measurements *runtime.Channel[*nmtypes.Measurement]
}

func NewSignal(ctx context.Context, thesis *types.Thesis, bus *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](temporal.Path),
	}

	signal.measurements = runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	)
	runtime.ChannelOf[kraken.TickerData](
		bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol },
	).Subscribe(signal.Name(), signal.Step)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceCorrelation) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceCorrelation }

// Step processes one ready symbol cut. The transport workspace preserves
// order for this symbol while allowing every other symbol to advance.
func (signal *Signal) Step(ticker kraken.TickerData) error {
	input := nmtypes.Frame{}
	input.Put(nmtypes.SampleValue, ticker.Last.Float64())
	input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
	_, err := signal.number.Step(ticker.Symbol, input)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"correlation: path failed for "+ticker.Symbol,
			err,
		))
	}

	output, reduced, err := signal.number.CrossSection(
		ticker.Symbol,
		nmcorrelation.Hayashi,
		nmcorrelation.Cohort,
		algo.Correlation(),
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"correlation: cohort failed for "+ticker.Symbol,
			err,
		))
	}

	ready, _ := output.Get(nmcorrelation.SymbolReady)
	separation := 0.0
	measured := reduced && ready != 0

	if measured {
		separation, _ = output.Get(algo.SymbolHypothesisSeparation)
	}

	signal.measurements.Publish(signal.measurement(
		ticker.Symbol,
		ticker.Timestamp,
		output,
		measured,
		separation,
		signal.support(ticker.Symbol),
	))
	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
