package liquidity

import (
	"context"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/runtime"
)

var outputKeys = []string{
	"best_bid_price",
	"best_ask_price",
	"touch_quantity:bid",
	"touch_quantity:ask",
	"touch_notional:bid",
	"touch_notional:ask",
	"midpoint",
	"spread",
	"relative_spread",
	"two_sided_touch_notional",
	"touch_notional_imbalance",
	"touch_notional_baseline:bid",
	"touch_notional_baseline:ask",
	"relative_spread_baseline",
	"depth_ratio:bid",
	"depth_ratio:ask",
	"spread_ratio",
	"depth_divergence:bid",
	"depth_divergence:ask",
	"spread_divergence",
	"depth_noise_scale:bid",
	"depth_noise_scale:ask",
	"spread_noise_scale",
	"depth_zscore:bid",
	"depth_zscore:ask",
	"spread_zscore",
	"divergence_velocity:bid",
	"divergence_velocity:ask",
	"spread_divergence_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type Signal struct {
	*runtime.System
	books    broker.BookSource
	pipeline core.Primitive
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:    books,
		pipeline: distribution.NewLiquidity(),
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", nil))
		return nil
	}

	var bid, ask, bidQty, askQty float64
	var found bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		b := book.BestBid()
		a := book.BestAsk()

		if b == nil || a == nil || b.Price == nil || a.Price == nil ||
			b.Quantity == nil || a.Quantity == nil {
			return
		}

		bid = kraken.Float64(b.Price)
		bidQty = kraken.Float64(b.Quantity)
		ask = kraken.Float64(a.Price)
		askQty = kraken.Float64(a.Quantity)
		found = true
	})

	if !found {
		return nil
	}

	touch := map[string]float64{
		"bid":     bid,
		"ask":     ask,
		"bid_qty": bidQty,
		"ask_qty": askQty,
	}

	for _, value := range touch {
		if !broker.ValidTouchValue(value) {
			signal.Error(broker.InvalidTouch("liquidity", prior.Label, touch))
			return nil
		}
	}

	if ask <= bid {
		signal.Error(broker.CrossedTouch("liquidity", prior.Label, bid, ask))
		return nil
	}

	if bidQty <= 0 || askQty <= 0 {
		return nil
	}

	atNano := float64(prior.At.UnixNano())

	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"liquidity",
			prior.Label,
			data.NewValue(bid, ask, bidQty, askQty, atNano),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}

		if index >= len(outputKeys) {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.liquidity] overflow",
				nil,
			))
			return nil
		}

		output[outputKeys[index]] = *(*float64)(ptr)
		index++
	}

	return prior.Next(signal.Name(), output)
}
