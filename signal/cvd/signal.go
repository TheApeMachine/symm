package cvd

import (
	"context"
	"fmt"
	"iter"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
BookSource is the narrow dependency CVD needs from the venue: read the resident
book for one symbol. It is an interface so tests can inject a deterministic book
instead of a live websocket API.
*/
type BookSource interface {
	Book(symbol string, read func(*spotbook.Book))
}

/*
Signal is the Absorption perspective, measuring signed aggressor flow against
price response. Categories belong in logic; this signal emits numerical scores
only.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    BookSource
	algo   *algorithm.TradeFlowSample
	flow   *equation.Flow
}

/*
NewSignal creates the CVD perspective with independent rolling state for each
symbol so one market's aggressor history cannot leak into another's evidence.
*/
func NewSignal(
	ctx context.Context,
	api BookSource,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
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

/*
flowHistoryCapacity mirrors the trade-flow sample's per-symbol history bound;
it is the denominator of measurement maturity.
*/
const flowHistoryCapacity = 128

func (signal *Signal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		for trade := range symbol.MarketTrades(types.SourceCVD) {
			var responsePrice float64

			responsePrice = trade.Price.Float64()

			if signal.api != nil {
				signal.api.Book(symbol.Symbol, func(book *spotbook.Book) {
					if book == nil {
						return
					}

					// Get the response price from the book's best bid and ask.
					bestBid := book.BestBid()
					bestAsk := book.BestAsk()

					if bestBid != nil && bestAsk != nil && bestBid.Price != nil && bestAsk.Price != nil {
						responsePrice = bestBid.Price.Add(
							bestAsk.Price,
						).Div(
							decimal.NewFromInt64(2),
						).Float64()
					}
				})
			}

			input, _, err := signal.algo.Measure(algorithm.TradeFlowInput{
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

			// The equation's own boundary is defined output: a single price
			// yields the balance reading, so every classified trade emits.
			output, err := signal.flow.Measure(input)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("cvd: flow [%s]", err.Error()),
					err,
				))
				continue
			}

			if !yield(signal.frame(symbol, trade, responsePrice, input, output)) {
				return
			}
		}
	}
}

/*
frame materializes one trade's flow evidence as a measurement. Partial output
from the equation's first-observation boundary carries the zero scores it did
not define rather than suppressing the row.
*/
func (signal *Signal) frame(
	symbol *types.Symbol,
	trade kraken.TradeData,
	responsePrice float64,
	input equation.FlowInput,
	output equation.FlowOutput,
) *types.Measurement {
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

	separation, separationReady := types.MeasurementHypothesisSeparation(types.SourceCVD, metrics)

	if !separationReady {
		separation = 0
	}

	metrics[types.MetricKey(types.MetricHypothesisSeparation, types.SideNone)] = types.MetricSample{
		Raw:        separation,
		Normalized: &separation,
		Unit:       types.UnitDimensionless,
	}

	return &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceCVD,
		Symbol:   symbol.Symbol,
		Tick:     symbol.Tick,
		At:       trade.Timestamp,
		Maturity: float64(input.TradeCount) / flowHistoryCapacity,
		Metadata: map[string]float64{
			"trade_price":    trade.Price.Float64(),
			"trade_quantity": trade.Qty,
		},
		Metrics: metrics,
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
