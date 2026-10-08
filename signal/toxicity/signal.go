package toxicity

import (
	"context"
	"time"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
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
	pipeline core.Primitive
}

func buildToxicityMathPipeline() core.Primitive {
	return nomagique.NewNumber(
		// Stage 1: Spread 4 slice inputs + passthrough 2 scalar inputs
		data.NewValue[core.Primitive](
			nomagique.NewNumber(
				data.NewSlice(0, 4),
				transport.NewParallel(
					// Branch 0: prices [bid, ask, prevBid, prevAsk] (indices 0..3)
					nomagique.NewNumber(transport.NewSpread[float64]()),
					// Branch 1: touchQuantities [bidQty, askQty, prevBidQty, prevAskQty] (indices 4..7)
					nomagique.NewNumber(transport.NewSpread[float64]()),
					// Branch 2: fills [cumBracket, matchedBid, matchedAsk, cumFillBid, cumFillAsk, fillFracBid, fillFracAsk] (indices 8..14)
					nomagique.NewNumber(transport.NewSpread[float64]()),
					// Branch 3: dispositions [retreatedBid, retreatedAsk, retreatFractionBid, retreatFractionAsk, netWithdrawnBid, netWithdrawnAsk, netReplenishedBid, netReplenishedAsk, logChangeBid, logChangeAsk] (indices 15..24)
					nomagique.NewNumber(transport.NewSpread[float64]()),
				),
			),
			data.NewSlice(4, 6), // 25: timeDelta, 26: atNano
		),
		// Stage 2: Fractions and Rates (yields 27..40, total 41 values)
		data.NewValue[core.Primitive](
			data.NewSlice(0, 27),
			nomagique.NewNumber(
				data.NewSelect(
					13, 4, // 27: touch_fill_fraction:bid = fillFracBid / bidQty
					14, 5, // 28: touch_fill_fraction:ask = fillFracAsk / askQty
					19, 6, // 29: net_withdrawal_fraction:bid = netWithdrawnBid / prevBidQty
					20, 7, // 30: net_withdrawal_fraction:ask = netWithdrawnAsk / prevAskQty
					21, 6, // 31: net_replenishment_fraction:bid = netReplenishedBid / prevBidQty
					22, 7, // 32: net_replenishment_fraction:ask = netReplenishedAsk / prevAskQty
					11, 25, // 33: touch_fill_rate:bid = cumFillBid / timeDelta
					12, 25, // 34: touch_fill_rate:ask = cumFillAsk / timeDelta
					15, 25, // 35: retreat_rate:bid = retreatedBid / timeDelta
					16, 25, // 36: retreat_rate:ask = retreatedAsk / timeDelta
					19, 25, // 37: net_withdrawal_rate:bid = netWithdrawnBid / timeDelta
					20, 25, // 38: net_withdrawal_rate:ask = netWithdrawnAsk / timeDelta
					21, 25, // 39: net_replenishment_rate:bid = netReplenishedBid / timeDelta
					22, 25, // 40: net_replenishment_rate:ask = netReplenishedAsk / timeDelta
				),
				data.NewBatch(2, 2),
				transport.NewParallel(
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
				),
			),
		),
		// Stage 3: Baselines, Velocities, and Constants (yields 41..62, total 63 values)
		data.NewValue[core.Primitive](
			data.NewSlice(0, 41),
			nomagique.NewNumber(
				data.NewSelect(
					27, 27, // 41, 42: fill_fraction center, scale (bid)
					28, 28, // 43, 44: fill_fraction center, scale (ask)
					29, 29, // 45, 46: withdrawal_fraction center, scale (bid)
					30, 30, // 47, 48: withdrawal_fraction center, scale (ask)
					17, 17, // 49, 50: retreat_fraction center, scale (bid)
					18, 18, // 51, 52: retreat_fraction center, scale (ask)
					31, 31, // 53, 54: replenishment_fraction center, scale (bid)
					32, 32, // 55, 56: replenishment_fraction center, scale (ask)
					27, 26, // 57: fill_fraction_velocity (bid)
					28, 26, // 58: fill_fraction_velocity (ask)
					29, 26, // 59: withdrawal_fraction_velocity (bid)
					30, 26, // 60: withdrawal_fraction_velocity (ask)
					0, 0, // 61: historical_path_distance (0 - 0 = 0.0)
					0, 0, // 62: historical_path_percentile (0 - 0 = 0.0)
				),
				data.NewBatch(2, 2),
				transport.NewParallel(
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
					nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
					nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
					nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
					nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
				),
			),
		),
		// Stage 4: Divergences (yields 63..66, total 67 values)
		data.NewValue[core.Primitive](
			data.NewSlice(0, 63),
			nomagique.NewNumber(
				data.NewSelect(
					27, 41, // 63: fill_fraction_divergence (bid)
					28, 43, // 64: fill_fraction_divergence (ask)
					29, 45, // 65: withdrawal_fraction_divergence (bid)
					30, 47, // 66: withdrawal_fraction_divergence (ask)
				),
				data.NewBatch(2, 2),
				transport.NewParallel(
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
				),
			),
		),
		// Stage 5: Z-Scores (yields 67..72, total 73 values)
		data.NewValue[core.Primitive](
			data.NewSlice(0, 67),
			nomagique.NewNumber(
				data.NewSelect(
					63, 42, // 67: fill_fraction_zscore (bid)
					64, 44, // 68: fill_fraction_zscore (ask)
					65, 46, // 69: withdrawal_fraction_zscore (bid)
					66, 48, // 70: withdrawal_fraction_zscore (ask)
					17, 50, // 71: retreat_fraction_zscore (bid)
					18, 52, // 72: retreat_fraction_zscore (ask)
				),
				data.NewBatch(2, 2),
				transport.NewParallel(
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
					nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
				),
			),
		),
		// Stage 6: Select exact outputKeys (63 values)
		data.NewSelect(
			0,  //  0: best_price:bid
			1,  //  1: best_price:ask
			4,  //  2: touch_quantity:bid
			5,  //  3: touch_quantity:ask
			4,  //  4: unfilled_residual_quantity:bid
			5,  //  5: unfilled_residual_quantity:ask
			8,  //  6: bracket_trade_quantity
			9,  //  7: matched_touch_trade_quantity:bid
			10, //  8: matched_touch_trade_quantity:ask
			11, //  9: touch_fill_quantity:bid
			12, // 10: touch_fill_quantity:ask
			27, // 11: touch_fill_fraction:bid
			28, // 12: touch_fill_fraction:ask
			41, // 13: fill_fraction_baseline:bid
			43, // 14: fill_fraction_baseline:ask
			63, // 15: fill_fraction_divergence:bid
			64, // 16: fill_fraction_divergence:ask
			67, // 17: fill_fraction_zscore:bid
			68, // 18: fill_fraction_zscore:ask
			57, // 19: fill_fraction_velocity:bid
			58, // 20: fill_fraction_velocity:ask
			2,  // 21: previous_best_price:bid
			3,  // 22: previous_best_price:ask
			6,  // 23: previous_touch_quantity:bid
			7,  // 24: previous_touch_quantity:ask
			23, // 25: touch_price_log_change:bid
			24, // 26: touch_price_log_change:ask
			15, // 27: retreated_quantity:bid
			16, // 28: retreated_quantity:ask
			17, // 29: retreat_fraction:bid
			18, // 30: retreat_fraction:ask
			35, // 31: retreat_rate:bid
			36, // 32: retreat_rate:ask
			19, // 33: net_withdrawn_quantity:bid
			20, // 34: net_withdrawn_quantity:ask
			29, // 35: net_withdrawal_fraction:bid
			30, // 36: net_withdrawal_fraction:ask
			37, // 37: net_withdrawal_rate:bid
			38, // 38: net_withdrawal_rate:ask
			21, // 39: net_replenished_quantity:bid
			22, // 40: net_replenished_quantity:ask
			31, // 41: net_replenishment_fraction:bid
			32, // 42: net_replenishment_fraction:ask
			39, // 43: net_replenishment_rate:bid
			40, // 44: net_replenishment_rate:ask
			33, // 45: touch_fill_rate:bid
			34, // 46: touch_fill_rate:ask
			45, // 47: withdrawal_fraction_baseline:bid
			47, // 48: withdrawal_fraction_baseline:ask
			65, // 49: withdrawal_fraction_divergence:bid
			66, // 50: withdrawal_fraction_divergence:ask
			69, // 51: withdrawal_fraction_zscore:bid
			70, // 52: withdrawal_fraction_zscore:ask
			59, // 53: withdrawal_fraction_velocity:bid
			60, // 54: withdrawal_fraction_velocity:ask
			49, // 55: retreat_fraction_baseline:bid
			51, // 56: retreat_fraction_baseline:ask
			71, // 57: retreat_fraction_zscore:bid
			72, // 58: retreat_fraction_zscore:ask
			53, // 59: replenishment_fraction_baseline:bid
			55, // 60: replenishment_fraction_baseline:ask
			61, // 61: historical_path_distance
			62, // 62: historical_path_percentile
		),
	)
}

func buildToxicityPipeline() core.Primitive {
	return nomagique.NewNumber(
		temporal.NewTouchDisposition(),
		data.NewValue[core.Primitive](
			nomagique.NewNumber(
				data.NewSlice(0, 6),
				buildToxicityMathPipeline(),
			),
			data.NewSlice(6, 7), // prevAtNano
		),
	)
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore",
				store.NewKV(),
				func(symbol string) core.Primitive {
					return buildToxicityPipeline()
				},
			),
		),
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

	touchVals := [4]float64{bid, ask, bidQty, askQty}
	tradeVals := [3]float64{price, qty, sideIndicator}
	atNano := float64(prior.At.UnixNano())

	output := make(map[string]float64)
	index := 0
	var prevAtNano float64

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				unsafe.Pointer(&touchVals),
				unsafe.Pointer(&tradeVals),
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
