package cvd

import (
	"context"
	"fmt"
	"iter"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
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
	algo   *algorithm.TradeFlowSample
	flow   *equation.Flow
	quotes *data.Series[[2]float64]
}

/*
NewSignal creates the CVD perspective with independent rolling state for each
symbol so one market's aggressor history cannot leak into another's evidence.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
) *Signal {
	return NewSignalWithQuotes(
		ctx,
		api,
		data.MustNewSeries[[2]float64](system.Cfg.PumpDump.Capacity),
	)
}

/*
NewSignalWithQuotes creates CVD state sharing one causal quote history with the
other tape calculations owned by the same shard.
*/
func NewSignalWithQuotes(
	ctx context.Context,
	api *websocket.API,
	quotes *data.Series[[2]float64],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		algo:   algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
		quotes: quotes,
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

func (signal *Signal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) iter.Seq[*types.Measurement] {
	return symbol.AlwaysYield(types.SourceCVD, signal.measure(symbol))
}

func (signal *Signal) measure(
	symbol *types.Symbol,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		for ticker := range symbol.MarketTickers(types.SourceCVD) {
			if ticker.Bid == nil || ticker.Ask == nil || ticker.Timestamp.IsZero() ||
				ticker.Bid.Sign() <= 0 || ticker.Ask.Cmp(ticker.Bid) <= 0 {
				continue
			}

			signal.quotes.Observe(
				ticker.Symbol,
				float64(ticker.Timestamp.Unix()),
				float64(ticker.Timestamp.Nanosecond()),
				[2]float64{ticker.Bid.Float64(), ticker.Ask.Float64()},
			)
		}

		for trade := range symbol.MarketTrades(types.SourceCVD) {
			sides, found := signal.quotes.AsOf(
				trade.Symbol,
				float64(trade.Timestamp.Unix()),
				float64(trade.Timestamp.Nanosecond()),
			)

			if !found {
				continue
			}

			responsePrice := (sides[0] + sides[1]) / 2
			input, ready, err := signal.algo.Measure(algorithm.TradeFlowInput{
				Symbol:        symbol.Symbol,
				Price:         trade.Price.Float64(),
				ResponsePrice: responsePrice,
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
					Raw:  responsePrice,
					Unit: types.UnitQuoteCurrency,
				},
			}

			separation, ok := types.MeasurementHypothesisSeparation(types.SourceCVD, metrics)

			if ok {
				metrics[types.MetricKey(types.MetricHypothesisSeparation, types.SideNone)] = types.MetricSample{
					Raw:        separation,
					Normalized: &separation,
					Unit:       types.UnitDimensionless,
				}
			}

			if !ready {
				continue
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

			if !yield(measurement) {
				return
			}
		}
	}
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
