package liquidity

import (
	"context"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
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

func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	snapshot, err := broker.ReadSnapshot(value)
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

	feeRate := data.Attribute[float64](value.Layers[0], "fee_rate")
	var frictionHurdle float64
	if feeRate > 0 {
		frictionHurdle = relSpread + 2.0*feeRate
	}

	return data.AppendVector(value, []float64{
		bid,
		ask,
		relSpread,
		feeRate,
		frictionHurdle,
	}, signal.keys...)
}
