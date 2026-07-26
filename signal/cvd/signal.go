package cvd

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Absorption perspective, measuring signed aggressor flow against
price response. Categories belong in logic; this signal emits numerical scores
only.
*/
type Signal struct {
	*types.Actor
	thesis    *types.Thesis
	ctx       context.Context
	cancel    context.CancelFunc
	sample    *algorithm.TradeFlowSample
	flow      *equation.Flow
	midpoints map[string]float64
	ui        chan []byte
}

/*
NewSignal creates the CVD perspective with independent rolling state for each
symbol so one market's aggressor history cannot leak into another's evidence.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		sample:    algorithm.NewTradeFlowSample(),
		flow:      equation.NewFlow(),
		midpoints: make(map[string]float64),
		ui:        ui,
	}

	signal.Actor = types.NewActor(ctx, "cvd", map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
		"trade":  {Topic: "thesis", Fn: signal.onTrade},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCVD)
}

/*
Initialize wires ticker and trade ingress from Live. Book floods are unused by
CVD (midpoints come from tickers) and must not fill a dead subscription.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceCVD, Status: types.SignalSkip}
	}

	if len(measurements) > 0 {
		signal.thesis.Publish(types.SourceCVD, measurements)
		return types.SignalResult{Source: types.SourceCVD, Measurements: measurements, Status: types.SignalReady}
	}

	return types.SignalResult{Source: types.SourceCVD, Status: types.SignalSkip}
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data
	measurements, err := signal.Calculate(nil, rows, nil)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceCVD, Status: types.SignalSkip}
	}

	if len(measurements) > 0 {
		signal.thesis.Publish(types.SourceCVD, measurements)
		return types.SignalResult{Source: types.SourceCVD, Measurements: measurements, Status: types.SignalReady}
	}

	return types.SignalResult{Source: types.SourceCVD, Status: types.SignalSkip}
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	out := make([]*types.Measurement, 0, len(trades))
	uiOut := make([]*types.Measurement, 0)

	for _, row := range tickers {
		if row.Symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if row.Bid == nil || row.Ask == nil {
			continue
		}

		if row.Bid.Sign() <= 0 || row.Ask.Cmp(row.Bid) <= 0 {
			continue
		}

		signal.midpoints[row.Symbol] = (row.Bid.Float64() + row.Ask.Float64()) / 2
	}

	for _, row := range trades {
		if row.Symbol == "" || row.Price.Sign() <= 0 || row.Qty <= 0 {
			continue
		}

		midpoint, exists := signal.midpoints[row.Symbol]

		if !exists {
			continue
		}

		measurements, err := signal.measureTrade(row, midpoint)

		if err != nil {
			continue
		}

		out = append(out, measurements...)

		if row.Symbol == types.Focus() {
			uiOut = append(uiOut, measurements...)
		}
	}

	if len(uiOut) > 0 {
		select {
		case signal.ui <- datura.Map[any]{
			"measurements": uiOut,
		}.Marshal():
		default:
			errnie.Error(errnie.Err(
				errnie.TooManyRequests,
				"wire: ui channel saturated; dropped measurements",
				nil,
			))
		}
	}

	return out, nil
}

/*
measureTrade separates execution notional from midpoint response before
classifying one aggressor observation through the adaptive CVD window.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
	midpoint float64,
) ([]*types.Measurement, error) {
	input, ready, maturity, err := signal.sample.Measure(algorithm.TradeFlowInput{
		Symbol:        row.Symbol,
		Price:         row.Price.Float64(),
		ResponsePrice: midpoint,
		Quantity:      row.Qty,
		Side:          row.Side,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.flow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	return signal.cvdMeasurements(row, output, maturity), nil
}

/*
cvdMeasurements maps signed-flow output into one source×symbol row whose
Metrics map preserves each reading's unit and normalization.
*/
func (signal *Signal) cvdMeasurements(
	row kraken.TradeData,
	output equation.FlowOutput,
	maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	scale := types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    row.Timestamp,
		Through: row.Timestamp,
	}
	measurement := &types.Measurement{
		Source:   types.SourceCVD,
		Symbol:   row.Symbol,
		At:       row.Timestamp,
		Maturity: maturity,
		Validity: validity,
		Metrics:  make(map[string]types.MetricSample, 7),
		Scale:    scale,
	}
	measurement.Metrics[types.MetricKey(types.MetricAbsorption, types.SideNone)] = types.MetricSample{Raw: output.Absorption, Normalized: types.NormalizeFinite(output.Absorption), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricDrive, types.SideNone)] = types.MetricSample{Raw: output.Drive, Normalized: types.NormalizeFinite(output.Drive), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricBalance, types.SideNone)] = types.MetricSample{Raw: output.Balance, Normalized: types.NormalizeFinite(output.Balance), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStarvation, types.SideNone)] = types.MetricSample{Raw: output.Starvation, Normalized: types.NormalizeFinite(output.Starvation), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: output.Value, Normalized: types.NormalizeFinite(output.Value), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNetFraction, types.SideNone)] = types.MetricSample{Raw: output.NetFraction, Normalized: types.NormalizeFinite(output.NetFraction), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNet, types.SideNone)] = types.MetricSample{Raw: output.Net, Unit: types.UnitQuoteCurrency}

	return []*types.Measurement{measurement}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
