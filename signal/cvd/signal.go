package cvd

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
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

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	trades := signal.trade.cache
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

		out = append(out,
			types.ObservationMeasurement(
				types.SourceCVD, types.CVD, types.MetricAbsorption,
				types.SubjectAggressorFlow, row.Symbol, row.Timestamp,
				types.UnitDimensionless, output.Absorption, maturity,
			),
			types.ObservationMeasurement(
				types.SourceCVD, types.CVD, types.MetricDrive,
				types.SubjectAggressorFlow, row.Symbol, row.Timestamp,
				types.UnitDimensionless, output.Drive, maturity,
			),
			types.ObservationMeasurement(
				types.SourceCVD, types.CVD, types.MetricBalance,
				types.SubjectAggressorFlow, row.Symbol, row.Timestamp,
				types.UnitDimensionless, output.Balance, maturity,
			),
			types.ObservationMeasurement(
				types.SourceCVD, types.CVD, types.MetricStarvation,
				types.SubjectAggressorFlow, row.Symbol, row.Timestamp,
				types.UnitDimensionless, output.Starvation, maturity,
			),
			types.ObservationMeasurement(
				types.SourceCVD, types.CVD, types.MetricStrength,
				types.SubjectAggressorFlow, row.Symbol, row.Timestamp,
				types.UnitDimensionless, output.Value, maturity,
			),
			types.ObservationMeasurement(
				types.SourceCVD, types.CVD, types.MetricNetFraction,
				types.SubjectAggressorFlow, row.Symbol, row.Timestamp,
				types.UnitDimensionless, output.NetFraction, maturity,
			),
			types.ObservationMeasurement(
				types.SourceCVD, types.CVD, types.MetricNet,
				types.SubjectAggressorFlow, row.Symbol, row.Timestamp,
				types.UnitQuoteCurrency, output.Net, maturity,
			),
		)
	}

	signal.trade.cache = signal.trade.cache[:0]

	thesis.Signals.Store("trades", trades)
	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
