package cvd

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
	api    *websocket.API
	ui     chan []byte
	algo   *algorithm.TradeFlowSample
	flow   *equation.Flow
}

/*
NewSignal creates the CVD perspective with independent rolling state for each
symbol so one market's aggressor history cannot leak into another's evidence.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		ui:     ui,
		algo:   algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCVD)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceCVD
}

func (signal *Signal) Measure(symbol *types.Symbol) []*types.Measurement {
	measurements := make([]*types.Measurement, 0)

	for trade := range symbol.MarketTrades(types.SourceCVD) {
		input, ready, err := signal.algo.Measure(algorithm.TradeFlowInput{
			Symbol:        symbol.Symbol,
			Price:         trade.Price.Float64(),
			ResponsePrice: trade.Price.Float64(),
			Quantity:      trade.Qty,
			Side:          trade.Side,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("cvd: trade-flow-sample [%s]", err.Error()),
				err,
			))
			continue
		}

		output, err := signal.flow.Measure(input)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("cvd: flow [%s]", err.Error()),
				err,
			))
			continue
		}

		metrics := map[string]types.MetricSample{
			types.MetricKey(types.MetricAbsorption, types.SideNone): {
				Raw:        output.Absorption,
				Normalized: &output.Absorption,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricDrive, types.SideNone): {
				Raw:        output.Drive,
				Normalized: &output.Drive,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricBalance, types.SideNone): {
				Raw:        output.Balance,
				Normalized: &output.Balance,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStarvation, types.SideNone): {
				Raw:        output.Starvation,
				Normalized: &output.Starvation,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideNone): {
				Raw:        output.Value,
				Normalized: &output.Value,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricNetFraction, types.SideNone): {
				Raw:        output.NetFraction,
				Normalized: &output.NetFraction,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricNet, types.SideNone): {
				Raw:  output.Net,
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTradePrice, types.SideNone): {
				Raw:  trade.Price.Float64(),
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTradeQuantity, types.SideNone): {
				Raw:  trade.Qty,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricMidpoint, types.SideNone): {
				Raw:  trade.Price.Float64(),
				Unit: types.UnitQuoteCurrency,
			},
		}

		snr, ok := types.MeasurementSignalNoiseRatio(types.SourceCVD, metrics)

		if ok {
			metrics[types.MetricKey(types.MetricSNR, types.SideNone)] = types.MetricSample{
				Raw:        snr,
				Normalized: &snr,
				Unit:       types.UnitDimensionless,
			}
		}

		measurement := &types.Measurement{
			ID:     uuid.NewString(),
			Source: types.SourceCVD,
			Symbol: symbol.Symbol,
			Tick:   symbol.Tick,
			At:     trade.Timestamp,
			Metadata: map[string]float64{
				"trade_price":    trade.Price.Float64(),
				"trade_quantity": trade.Qty,
			},
			Metrics: metrics,
		}

		if ready {
			measurements = append(measurements, measurement)
		}
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
