package cvd

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
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
	out := datura.Map[datura.Map[*decimal.Decimal]]{}

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

		out[row.Symbol] = datura.Map[*decimal.Decimal]{
			"absorption":  decimal.NewFromFloat64(output.Absorption),
			"drive":       decimal.NewFromFloat64(output.Drive),
			"balance":     decimal.NewFromFloat64(output.Balance),
			"starvation":  decimal.NewFromFloat64(output.Starvation),
			"strength":    decimal.NewFromFloat64(output.Value),
			"net":         decimal.NewFromFloat64(output.Net),
			"netFraction": decimal.NewFromFloat64(output.NetFraction),
			"category":    decimal.NewFromFloat64(output.Category),
			"maturity":    decimal.NewFromFloat64(maturity),
		}
	}

	signal.trade.cache = signal.trade.cache[:0]

	thesis.Signals.Store("trades", trades)
	thesis.Measurements.Store("cvd", out)

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
