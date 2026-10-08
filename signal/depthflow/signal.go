package depthflow

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
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/runtime"
	"unsafe"
)

var outputKeys = []string{
	"book_notional:bid",
	"book_notional:ask",
	"book_notional",
	"observed_notional:bid",
	"observed_notional:ask",
	"observed_notional",
	"book_imbalance",
	"observed_notional_imbalance",
	"touch_imbalance",
	"imbalance_resolution_gap",
	"imbalance_resolution_distance",
	"added_notional:bid",
	"removed_notional:bid",
	"net_displayed_flow:bid",
	"added_notional:ask",
	"removed_notional:ask",
	"net_displayed_flow:ask",
	"flow_activity_imbalance",
	"book_imbalance_baseline",
	"book_imbalance_divergence",
	"book_imbalance_zscore",
	"resolution_gap_baseline",
	"resolution_gap_divergence",
	"resolution_gap_zscore",
	"book_imbalance_velocity",
	"resolution_gap_velocity",
	"added_notional_rate:bid",
	"added_notional_rate:ask",
	"removed_notional_rate:bid",
	"removed_notional_rate:ask",
	"net_displayed_flow_rate:bid",
	"net_displayed_flow_rate:ask",
	"book_turnover_rate",
	"net_book_change_rate",
	"signed_net_displayed_flow_rate",
	"turnover_baseline",
	"turnover_divergence",
	"turnover_zscore",
	"turnover_ratio",
	"net_book_change_rate_baseline",
	"net_book_change_rate_divergence",
	"net_book_change_rate_zscore",
	"signed_net_displayed_flow_rate_baseline",
	"signed_net_displayed_flow_rate_divergence",
	"signed_net_displayed_flow_rate_zscore",
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
			distribution.NewDepthFlow(),
		),
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", nil))
		return nil
	}

	var bids, asks [][2]float64
	var crossedBid, crossedAsk float64
	ok := false
	crossed := false

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bid := book.BestBid()
		ask := book.BestAsk()

		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}

		bidPrice := kraken.Float64(bid.Price)
		askPrice := kraken.Float64(ask.Price)

		if askPrice <= bidPrice {
			crossed = true
			crossedBid = bidPrice
			crossedAsk = askPrice
			return
		}

		for cursor, count := bid, 0; cursor != nil && count < 100; cursor, count = cursor.Lower, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 {
				continue
			}

			bids = append(bids, [2]float64{price, qty})
		}

		for cursor, count := ask, 0; cursor != nil && count < 100; cursor, count = cursor.Higher, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 {
				continue
			}

			asks = append(asks, [2]float64{price, qty})
		}

		ok = len(bids) > 0 && len(asks) > 0
	})

	if crossed {
		signal.Error(broker.CrossedTouch("depthflow", prior.Label, crossedBid, crossedAsk))
		return nil
	}

	if !ok {
		return nil
	}

	pairs := [2][][2]float64{bids, asks}
	atNano := float64(prior.At.UnixNano())

	output := make(map[string]float64)
	index := 0
	var prevAtNano float64

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewPointers(
				unsafe.Pointer(&pairs),
				unsafe.Pointer(&atNano),
			),
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
