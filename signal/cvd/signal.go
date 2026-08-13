package cvd

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
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
	quotes *types.QuoteHistory
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
	return NewSignalWithQuotes(
		ctx,
		api,
		ui,
		types.NewQuoteHistory(system.Cfg.PumpDump.Capacity),
	)
}

/*
NewSignalWithQuotes creates CVD state sharing one causal quote history with the
other tape calculations owned by the same shard.
*/
func NewSignalWithQuotes(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	quotes *types.QuoteHistory,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		ui:     ui,
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

func (signal *Signal) Measure(symbol *types.Symbol, _ ...int64) []*types.Measurement {
	utils.PublishPriority(signal.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourceCVD), "running",
	)))

	defer utils.PublishPriority(signal.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourceCVD), "done",
	)))

	measurements := make([]*types.Measurement, 0)

	for ticker := range symbol.MarketTickers(types.SourceCVD) {
		signal.quotes.Observe(ticker)
	}

	for trade := range symbol.MarketTrades(types.SourceCVD) {
		quote, found := signal.quotes.At(trade.Symbol, trade.Timestamp)

		if !found {
			continue
		}

		responsePrice := (quote.Bid.Float64() + quote.Ask.Float64()) / 2
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
