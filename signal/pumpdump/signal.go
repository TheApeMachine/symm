package pumpdump

import (
	"context"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

var outputKeys = []string{
	"trade_price",
	"trade_quantity",
	"trade_notional",
	"trade_interval_seconds",
	"volume_bar_target_quantity",
	"volume_bar_quantity",
	"volume_bar_notional",
	"volume_bar_trade_count",
	"volume_bar_duration",
	"volume_rate",
	"notional_rate",
	"trade_rate",
	"completed_bars",
	"notional_rate_baseline",
	"notional_rate_ratio",
	"notional_rate_divergence",
	"notional_rate_zscore",
	"notional_rate_velocity",
	"best_bid",
	"best_ask",
	"midpoint",
	"spread",
	"relative_spread",
	"relative_spread_baseline",
	"spread_ratio",
	"spread_divergence",
	"spread_zscore",
	"spread_divergence_velocity",
	"midpoint:from",
	"midpoint:at",
	"midpoint_log_return",
	"midpoint_return_rate",
	"positive_midpoint_return",
	"negative_midpoint_return",
	"midpoint_return_baseline",
	"midpoint_return_divergence",
	"midpoint_return_zscore",
	"midpoint_return_velocity",
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
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore",
				store.NewKV(),
				nomagique.NewNumber(
					temporal.NewActivity(1.0),
				),
				data.NewMessage(data.WRITE, "symbolstore", "pumpdump_state", data.NewValue[core.Primitive]()),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", nil))
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		signal.Error(errnie.Err(errnie.Validation, "[pumpdump] price is required", nil))
		return nil
	}
	price := priceEntry.Metric.Raw

	qtyEntry := data.Pull(prior.Read("qty"))
	if qtyEntry == nil || qtyEntry.Metric == nil || qtyEntry.Metric.Raw <= 0 {
		return nil
	}
	qty := qtyEntry.Metric.Raw

	var bid, ask float64
	var hasBook bool

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
		ask = kraken.Float64(a.Price)
		hasBook = true
	})

	if hasBook {
		touch := map[string]float64{
			"bid": bid,
			"ask": ask,
		}

		for _, value := range touch {
			if !broker.ValidTouchValue(value) {
				signal.Error(broker.InvalidTouch("pumpdump", prior.Label, touch))
				return nil
			}
		}

		if ask <= bid {
			signal.Error(broker.CrossedTouch("pumpdump", prior.Label, bid, ask))
			return nil
		}
	}

	atNanos := float64(prior.At.UnixNano())
	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(price, qty, atNanos, bid, ask),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}

		if index < len(outputKeys) {
			output[outputKeys[index]] = *(*float64)(ptr)
		}

		if index == len(outputKeys) {
			outBarStart := *(*float64)(ptr)
			if outBarStart > 0 {
				prior.From = time.Unix(0, int64(outBarStart))
			}
		}
		index++
	}

	return prior.Next(signal.Name(), output)
}
