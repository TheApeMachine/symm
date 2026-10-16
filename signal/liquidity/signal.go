package liquidity

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Signal struct {
	*runtime.System
	keys []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		keys: []string{
			"best_bid_price",
			"best_ask_price",
			"relative_spread",
			"fee_rate",
			"friction_hurdle",
		},
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)
	return signal
}

/*
Step processes the book of every distinct symbol traded in the tick's frame;
each symbol writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	trades, err := data.AttributeOk[[]kraken.TradeData](value.Layers[0], "data")

	if err != nil {
		errnie.Error(err)
		return value
	}

	for _, symbol := range broker.Symbols(trades) {
		signal.step(value, symbol)
	}

	return value
}

func (signal *Signal) step(value *data.Value, symbol string) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	snapshot, err := broker.ReadSnapshot(value, symbol)
	if err != nil || snapshot == nil || len(snapshot.Bids) == 0 || len(snapshot.Asks) == 0 {
		return value
	}

	bestBid, bestAsk := snapshot.Touch()
	bid := bestBid.Price.Float64()
	ask := bestAsk.Price.Float64()

	midpoint := (bid + ask) / 2.0
	var relSpread float64
	if midpoint > 0 {
		relSpread = (ask - bid) / midpoint
	}

	feeRate := data.Attribute[float64](broker.SymbolLayer(value, "price", symbol), "feeRate")
	var frictionHurdle float64
	if feeRate > 0 {
		frictionHurdle = relSpread + 2.0*feeRate
	}

	return data.AppendSymbolVector(value, symbol, []float64{
		bid,
		ask,
		relSpread,
		feeRate,
		frictionHurdle,
	}, signal.keys...)
}
