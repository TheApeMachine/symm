package leadlag

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
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
	pair         nmtypes.Primitive
	measurements *runtime.Channel[*nmtypes.Measurement]
}

func NewSignal(ctx context.Context, thesis *types.Thesis, bus *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](temporal.Path),
		pair:   algo.LeadLag(),
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

func (signal *Signal) Name() string           { return string(types.SourceLeadLag) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceLeadLag }

// Step processes one ready symbol cut. The transport workspace preserves
// order for this symbol while allowing every other symbol to advance.
func (signal *Signal) Step(ticker kraken.TickerData) error {
	price, observed, err := tickerPrice(ticker)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"leadlag: invalid ticker for "+ticker.Symbol,
			err,
		))
	}

	if !observed {
		return nil
	}

	anchor, _, _, hasAnchor, err := signal.number.ArgMax(
		correlation.Return,
		correlation.SymbolMagnitude,
		correlation.SymbolReady,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"leadlag: anchor selection failed",
			err,
		))
	}

	input := nmtypes.Frame{}
	input.Put(nmtypes.SampleValue, price)
	input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
	_, err = signal.number.Step(ticker.Symbol, input)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"leadlag: path failed for "+ticker.Symbol,
			err,
		))
	}

	if !hasAnchor || anchor == ticker.Symbol {
		types.PublishMeasurement(signal.thesis, signal.measurements, ticker.Symbol, signal.neutralMeasurement(
			ticker.Symbol, ticker.Timestamp, price,
		))

		return nil
	}

	anchorPath, hasAnchorPath := signal.number.Project(anchor)
	followerPath, hasFollowerPath := signal.number.Project(ticker.Symbol)

	if !hasAnchorPath || !hasFollowerPath {
		types.PublishMeasurement(signal.thesis, signal.measurements, ticker.Symbol, signal.neutralMeasurement(
			ticker.Symbol, ticker.Timestamp, price,
		))

		return nil
	}

	_, output, err := signal.pair(anchorPath, followerPath)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"leadlag: pair failed for "+ticker.Symbol,
			err,
		))
	}

	ready, _ := output.Get(correlation.SymbolLeadLagReady)

	types.PublishMeasurement(signal.thesis, signal.measurements, ticker.Symbol, signal.measurement(
		ticker.Symbol,
		anchor,
		ticker.Timestamp,
		anchorPath,
		followerPath,
		output,
		ready != 0,
	))
	return nil
}

func tickerPrice(ticker kraken.TickerData) (float64, bool, error) {
	if ticker.Last == nil {
		return 0, false, fmt.Errorf("leadlag: ticker requires a last price")
	}

	if ticker.Last.Sign() < 0 {
		return 0, false, fmt.Errorf("leadlag: ticker last price cannot be negative")
	}

	if ticker.Last.Sign() == 0 {
		return 0, false, nil
	}

	return ticker.Last.Float64(), true, nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
