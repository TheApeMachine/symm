package toxicity

import (
	"context"
	"math"
	"sync"
	"time"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
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
	pipeline *nomagique.Number
	history  sync.Map
}

type symbolHistory struct {
	hasPrev    bool
	prevBid    float64
	prevAsk    float64
	prevBidQty float64
	prevAskQty float64
	prevAt     time.Time
	cumBracket float64
	cumFillBid float64
	cumFillAsk float64
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					data.NewSelect(
						0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
						10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
						20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
						30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
						40, 41, 42, 43, 44, 45, 46, 47, 48, 49,
						50, 51, 52, 53, 54, 55, 56, 57, 58, 59,
						60, 61, 62,
					),
				),
				data.NewMessage(data.WRITE, "symbolstore", "toxicity_state", data.NewValue[core.Primitive]()),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", err))
	}

	return signal
}

func (signal *Signal) getHistory(symbol string) *symbolHistory {
	val, ok := signal.history.Load(symbol)
	if ok {
		return val.(*symbolHistory)
	}

	hist := &symbolHistory{}
	actual, _ := signal.history.LoadOrStore(symbol, hist)
	return actual.(*symbolHistory)
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

	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) || qty <= 0 || math.IsNaN(qty) || math.IsInf(qty, 0) {
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
			signal.Error(broker.InvalidTouch("toxicity", prior.Label, touch))
			return nil
		}
	}

	if ask <= bid {
		signal.Error(broker.CrossedTouch("toxicity", prior.Label, bid, ask))
		return nil
	}

	hist := signal.getHistory(prior.Label)

	hadPrev := hist.hasPrev
	prevBid := hist.prevBid
	prevAsk := hist.prevAsk
	prevBidQty := hist.prevBidQty
	prevAskQty := hist.prevAskQty
	prevAt := hist.prevAt
	rateReady := hadPrev && prior.At.After(prevAt)

	inBracket := (price >= bid) && (price <= ask)
	if inBracket {
		hist.cumBracket += qty
	}

	side := prior.Meta("side")
	var matchedBid, matchedAsk bool
	if side == "sell" && price == bid {
		hist.cumFillBid += qty
		matchedBid = true
	}
	if side == "buy" && price == ask {
		hist.cumFillAsk += qty
		matchedAsk = true
	}

	var touchFillFractionBid, touchFillFractionAsk float64
	if matchedBid && bidQty > 0 {
		touchFillFractionBid = hist.cumFillBid / bidQty
	}
	if matchedAsk && askQty > 0 {
		touchFillFractionAsk = hist.cumFillAsk / askQty
	}

	var touchPriceLogChangeBid, touchPriceLogChangeAsk float64
	var retreatedQtyBid, retreatedQtyAsk, retreatFractionBid, retreatFractionAsk float64
	var netWithdrawnBid, netWithdrawnAsk, netWithdrawalFractionBid, netWithdrawalFractionAsk float64
	var netReplenishedBid, netReplenishedAsk, netReplenishmentFractionBid, netReplenishmentFractionAsk float64

	if hadPrev {
		if prevBid > 0 {
			touchPriceLogChangeBid = math.Log(bid / prevBid)
		}
		if prevAsk > 0 {
			touchPriceLogChangeAsk = math.Log(ask / prevAsk)
		}

		if bid < prevBid {
			retreatFractionBid = 1.0
			retreatedQtyBid = prevBidQty
		} else if bid == prevBid {
			drop := prevBidQty - bidQty
			if drop > 0 {
				netWithdrawnBid = drop
				if prevBidQty > 0 {
					netWithdrawalFractionBid = drop / prevBidQty
				}
			} else if drop < 0 {
				netReplenishedBid = -drop
				if prevBidQty > 0 {
					netReplenishmentFractionBid = -drop / prevBidQty
				}
			}
		}

		if ask > prevAsk {
			retreatFractionAsk = 1.0
			retreatedQtyAsk = prevAskQty
		} else if ask == prevAsk {
			drop := prevAskQty - askQty
			if drop > 0 {
				netWithdrawnAsk = drop
				if prevAskQty > 0 {
					netWithdrawalFractionAsk = drop / prevAskQty
				}
			} else if drop < 0 {
				netReplenishedAsk = -drop
				if prevAskQty > 0 {
					netReplenishmentFractionAsk = -drop / prevAskQty
				}
			}
		}
	}

	var retreatRateBid, retreatRateAsk float64
	var netWithdrawalRateBid, netWithdrawalRateAsk float64
	var netReplenishmentRateBid, netReplenishmentRateAsk float64
	var touchFillRateBid, touchFillRateAsk float64

	if rateReady {
		elapsed := prior.At.Sub(prevAt).Seconds()
		if elapsed > 0 {
			retreatRateBid = retreatedQtyBid / elapsed
			retreatRateAsk = retreatedQtyAsk / elapsed
			netWithdrawalRateBid = netWithdrawnBid / elapsed
			netWithdrawalRateAsk = netWithdrawnAsk / elapsed
			netReplenishmentRateBid = netReplenishedBid / elapsed
			netReplenishmentRateAsk = netReplenishedAsk / elapsed
			touchFillRateBid = hist.cumFillBid / elapsed
			touchFillRateAsk = hist.cumFillAsk / elapsed
		}
	}

	rawMetrics := []float64{
		bid,
		ask,
		bidQty,
		askQty,
		bidQty,
		askQty,
		hist.cumBracket,
		hist.cumFillBid,
		hist.cumFillAsk,
		hist.cumFillBid,
		hist.cumFillAsk,
		touchFillFractionBid,
		touchFillFractionAsk,
		0, 0, 0, 0, 0, 0, 0, 0, // baselines, divergences, zscores, velocities
		prevBid,
		prevAsk,
		prevBidQty,
		prevAskQty,
		touchPriceLogChangeBid,
		touchPriceLogChangeAsk,
		retreatedQtyBid,
		retreatedQtyAsk,
		retreatFractionBid,
		retreatFractionAsk,
		retreatRateBid,
		retreatRateAsk,
		netWithdrawnBid,
		netWithdrawnAsk,
		netWithdrawalFractionBid,
		netWithdrawalFractionAsk,
		netWithdrawalRateBid,
		netWithdrawalRateAsk,
		netReplenishedBid,
		netReplenishedAsk,
		netReplenishmentFractionBid,
		netReplenishmentFractionAsk,
		netReplenishmentRateBid,
		netReplenishmentRateAsk,
		touchFillRateBid,
		touchFillRateAsk,
		0, 0, 0, 0, 0, 0, 0, 0, // withdrawal baselines etc.
		0, 0, 0, 0, // retreat baselines etc.
		0, 0, // replenishment baselines
		0, 0, // historical path distance, percentile
	}

	output := make(map[string]float64)

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				unsafe.Pointer(&rawMetrics),
			),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}
	}

	for i, key := range outputKeys {
		if i < len(rawMetrics) {
			output[key] = rawMetrics[i]
		}
	}

	hist.hasPrev = true
	hist.prevBid = bid
	hist.prevAsk = ask
	hist.prevBidQty = bidQty
	hist.prevAskQty = askQty
	hist.prevAt = prior.At

	out := prior.Next(signal.Name(), output)
	if rateReady {
		out.From = prevAt
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
