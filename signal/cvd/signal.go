package cvd

import (
	"container/ring"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Absorption perspective, measuring signed aggressor flow against
price response. Categories belong in logic; this signal emits numerical scores
only.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	trade  *Trade
	sample *algorithm.TradeFlowSample
	flow   *equation.Flow
}

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		trade:  NewTrade(ctx, api),
		sample: algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
	}
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	trades := make([]kraken.TradeData, 0)
	signal.trade.cache.Range(func(key, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				trades = append(trades, value.(kraken.TradeData))
			}
		})
		signal.trade.cache.Delete(key)

		return true
	})
	out := make([]*types.Measurement, 0, len(trades))

	for _, row := range trades {
		if row.Symbol == "" || row.Price.Sign() <= 0 || row.Qty <= 0 {
			continue
		}

		input, ready, maturity, err := signal.sample.Measure(algorithm.TradeFlowInput{
			Symbol:   row.Symbol,
			Price:    row.Price.Float64(),
			Quantity: row.Qty,
			Side:     row.Side,
		})

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		if !ready {
			continue
		}

		output, err := signal.flow.Measure(input)

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		out = append(out, signal.cvdMeasurements(row, output, maturity)...)
	}

	thesis.Signals.Store("trades", trades)
	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

/*
cvdMeasurements maps signed-flow output into typed CVD measurements while
preserving each metric's unit and normalization.
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
	specs := []struct {
		metric types.MetricType
		unit   types.MeasurementUnit
		value  float64
	}{
		{types.MetricAbsorption, types.UnitDimensionless, output.Absorption},
		{types.MetricDrive, types.UnitDimensionless, output.Drive},
		{types.MetricBalance, types.UnitDimensionless, output.Balance},
		{types.MetricStarvation, types.UnitDimensionless, output.Starvation},
		{types.MetricStrength, types.UnitDimensionless, output.Value},
		{types.MetricNetFraction, types.UnitDimensionless, output.NetFraction},
		{types.MetricNet, types.UnitQuoteCurrency, output.Net},
	}
	measurements := make([]*types.Measurement, 0, len(specs))

	for _, spec := range specs {
		var normalized *float64

		if spec.unit == types.UnitDimensionless {
			normalized = types.NormalizeFinite(spec.value)
		}

		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceCVD,
			Stream:     types.CVD,
			Metric:     spec.metric,
			Subject:    types.SubjectAggressorFlow,
			Symbol:     row.Symbol,
			At:         row.Timestamp,
			Unit:       spec.unit,
			Raw:        spec.value,
			Normalized: normalized,
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		})
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
