package toxicity

import (
	"context"
	"time"

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
	"best_price:bid",
	"best_price:ask",
	"touch_quantity:bid",
	"touch_quantity:ask",
	"unfilled_residual_quantity:bid",
	"unfilled_residual_quantity:ask",
	"bracket_trade_quantity",
	"matched_touch_trade_quantity:bid",
	"matched_touch_trade_quantity:ask",
	"touch_fill_quantity:bid",
	"touch_fill_quantity:ask",
	"touch_fill_fraction:bid",
	"touch_fill_fraction:ask",
	"fill_fraction_baseline:bid",
	"fill_fraction_baseline:ask",
	"fill_fraction_divergence:bid",
	"fill_fraction_divergence:ask",
	"fill_fraction_zscore:bid",
	"fill_fraction_zscore:ask",
	"fill_fraction_velocity:bid",
	"fill_fraction_velocity:ask",
	"previous_best_price:bid",
	"previous_best_price:ask",
	"previous_touch_quantity:bid",
	"previous_touch_quantity:ask",
	"touch_price_log_change:bid",
	"touch_price_log_change:ask",
	"retreated_quantity:bid",
	"retreated_quantity:ask",
	"retreat_fraction:bid",
	"retreat_fraction:ask",
	"retreat_rate:bid",
	"retreat_rate:ask",
	"net_withdrawn_quantity:bid",
	"net_withdrawn_quantity:ask",
	"net_withdrawal_fraction:bid",
	"net_withdrawal_fraction:ask",
	"net_withdrawal_rate:bid",
	"net_withdrawal_rate:ask",
	"net_replenished_quantity:bid",
	"net_replenished_quantity:ask",
	"net_replenishment_fraction:bid",
	"net_replenishment_fraction:ask",
	"net_replenishment_rate:bid",
	"net_replenishment_rate:ask",
	"touch_fill_rate:bid",
	"touch_fill_rate:ask",
	"withdrawal_fraction_baseline:bid",
	"withdrawal_fraction_baseline:ask",
	"withdrawal_fraction_divergence:bid",
	"withdrawal_fraction_divergence:ask",
	"withdrawal_fraction_zscore:bid",
	"withdrawal_fraction_zscore:ask",
	"withdrawal_fraction_velocity:bid",
	"withdrawal_fraction_velocity:ask",
	"retreat_fraction_baseline:bid",
	"retreat_fraction_baseline:ask",
	"retreat_fraction_zscore:bid",
	"retreat_fraction_zscore:ask",
	"replenishment_fraction_baseline:bid",
	"replenishment_fraction_baseline:ask",
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
		pipeline: distribution.NewToxicity(),
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", err))
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
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", nil))
		return nil
	}

	price, err := tradeValue(prior, "price")
	if err != nil {
		signal.Error(err)
		return nil
	}

	qty, err := tradeValue(prior, "qty")
	if err != nil {
		signal.Error(err)
		return nil
	}

	if price <= 0 || qty <= 0 {
		return nil
	}

	var bid, ask, bidQty, askQty float64
	var found bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bestBid := book.BestBid()
		bestAsk := book.BestAsk()

		if bestBid == nil || bestAsk == nil || bestBid.Price == nil || bestAsk.Price == nil ||
			bestBid.Quantity == nil || bestAsk.Quantity == nil {
			return
		}

		bid = kraken.Float64(bestBid.Price)
		bidQty = kraken.Float64(bestBid.Quantity)
		ask = kraken.Float64(bestAsk.Price)
		askQty = kraken.Float64(bestAsk.Quantity)
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
			signal.Error(broker.InvalidTouch("toxicity", prior.Label, touch))
			return nil
		}
	}

	if ask <= bid {
		signal.Error(broker.CrossedTouch("toxicity", prior.Label, bid, ask))
		return nil
	}

	sideIndicator := 0.0
	side := prior.Meta("side")

	if side == "buy" {
		sideIndicator = 1.0
	}

	if side == "sell" {
		sideIndicator = -1.0
	}

	atNano := float64(prior.At.UnixNano())

	output := make(map[string]float64)
	index := 0
	var prevAtNano float64

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"toxicity",
			prior.Label,
			data.NewValue(bid, ask, bidQty, askQty, price, qty, sideIndicator, atNano),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}

		if index < len(outputKeys) {
			output[outputKeys[index]] = *(*float64)(ptr)
		}

		if index == len(outputKeys) {
			prevAtNano = *(*float64)(ptr)
		}

		index++
	}

	out := prior.Next(signal.Name(), output)

	if prevAtNano > 0 {
		out.From = time.Unix(0, int64(prevAtNano))
	}

	return out
}

func tradeValue(prior *data.Measurement, key string) (float64, error) {
	entry := data.Pull(prior.Read(key))

	if entry != nil && entry.Err != nil {
		return 0, entry.Err
	}

	if entry == nil || entry.Metric == nil || entry.Metric.Label != key {
		return 0, errnie.Err(
			errnie.NotAcceptable, "[toxicity] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
