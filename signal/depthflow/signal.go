package depthflow

import (
	"context"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
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
			transport.NewAddressable(
				"symbolstore",
				store.NewKV(),
				nomagique.NewNumber(
					// Stage 1: Parallel branches across 4 slice inputs + passthrough of 4 scalar inputs
					data.NewValue[core.Primitive](
						nomagique.NewNumber(
							data.NewSlice(0, 4),
							transport.NewParallel(
								// Branch 0: bookNotionals [obsBid, obsAsk]
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(), // 0: obsBid, 1: obsAsk
										arithmetic.NewAdd(), // 2: totalNotional
										nomagique.NewNumber(
											data.NewValue[core.Primitive](
												arithmetic.NewSubtract(),
												arithmetic.NewAdd(),
											),
											arithmetic.NewDivide(),
										), // 3: book_imbalance = (obsBid - obsAsk) / (obsBid + obsAsk)
									),
								),
								// Branch 1: touchNotionals [touchBid, touchAsk]
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(), // 4: touchBid, 5: touchAsk
										arithmetic.NewAdd(), // 6: touchTotal
										nomagique.NewNumber(
											data.NewValue[core.Primitive](
												arithmetic.NewSubtract(),
												arithmetic.NewAdd(),
											),
											arithmetic.NewDivide(),
										), // 7: touch_imbalance = (touchBid - touchAsk) / (touchBid + touchAsk)
									),
								),
								// Branch 2: bidFlow [addedBid, removedBid]
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(),      // 8: addedBid, 9: removedBid
										arithmetic.NewSubtract(), // 10: netDisplayedFlowBid
										arithmetic.NewAdd(),      // 11: grossBidFlow
									),
								),
								// Branch 3: askFlow [addedAsk, removedAsk]
								nomagique.NewNumber(
									transport.NewSpread[float64](),
									data.NewValue[core.Primitive](
										transport.NewPass(),      // 12: addedAsk, 13: removedAsk
										arithmetic.NewSubtract(), // 14: netDisplayedFlowAsk
										arithmetic.NewAdd(),      // 15: grossAskFlow
									),
								),
							),
						),
						data.NewSlice(4, 8), // 16: timeDelta, 17: refExposure, 18: prevNotional, 19: atNano
					),
					// Stage 2: Gaps, Distances, Differences, and Rates (yields 20..31, total 32 values)
					data.NewValue[core.Primitive](
						data.NewSlice(0, 20),
						nomagique.NewNumber(
							data.NewSelect(
								7, 3, // 20: imbalance_resolution_gap = touchImbalance - bookImbalance
								7, 3, // 21: imbalance_resolution_distance = |touchImbalance - bookImbalance|
								10, 14, // 22: signedNetFlow = netBid - netAsk
								11, 15, // 23: bookActivity = grossBid + grossAsk
								2, 18, // 24: netBookChange = totalNotional - prevNotional
								8, 16, // 25: added_notional_rate:bid = addedBid / timeDelta
								12, 16, // 26: added_notional_rate:ask = addedAsk / timeDelta
								9, 16, // 27: removed_notional_rate:bid = removedBid / timeDelta
								13, 16, // 28: removed_notional_rate:ask = removedAsk / timeDelta
								10, 16, // 29: net_displayed_flow_rate:bid = netBid / timeDelta
								14, 16, // 30: net_displayed_flow_rate:ask = netAsk / timeDelta
								10, 14, // 31: flow_activity_imbalance = signedNetFlow / (|netBid| + |netAsk|)
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract(), calculus.NewAbsolute()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewAdd()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(
									data.NewUnpack(),
									data.NewValue[core.Primitive](
										arithmetic.NewSubtract(),
										nomagique.NewNumber(
											transport.NewParallel(
												calculus.NewAbsolute(),
												calculus.NewAbsolute(),
											),
											arithmetic.NewAdd(),
										),
									),
									arithmetic.NewDivide(),
								),
							),
						),
					),
					// Stage 3: Exposure-Based Rates (yields 32..34, total 35 values)
					data.NewValue[core.Primitive](
						data.NewSlice(0, 32),
						nomagique.NewNumber(
							data.NewSelect(
								23, 17, // 32: book_turnover_rate = bookActivity / refExposure
								24, 17, // 33: net_book_change_rate = netBookChange / refExposure
								22, 17, // 34: signed_net_displayed_flow_rate = signedNetFlow / refExposure
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
							),
						),
					),
					// Stage 4: Baselines, Velocities, and Path Constants (yields 35..48, total 49 values)
					data.NewValue[core.Primitive](
						data.NewSlice(0, 35),
						nomagique.NewNumber(
							data.NewSelect(
								3, 3, // 35, 36: book_imbalance center, scale
								20, 20, // 37, 38: resolution_gap center, scale
								32, 32, // 39, 40: turnover center, scale
								33, 33, // 41, 42: net_book_change center, scale
								34, 34, // 43, 44: signed_net center, scale
								3, 19, // 45: book_imbalance_velocity (val, atNano)
								20, 19, // 46: resolution_gap_velocity (val, atNano)
								0, 0, // 47: historical_path_distance (0 - 0 = 0.0)
								0, 0, // 48: historical_path_percentile (0 - 0 = 0.0)
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
							),
						),
					),
					// Stage 5: Divergences & Turnover Ratio (yields 49..54, total 55 values)
					data.NewValue[core.Primitive](
						data.NewSlice(0, 49),
						nomagique.NewNumber(
							data.NewSelect(
								3, 35, // 49: book_imbalance_divergence = book_imbalance - book_imbalance_baseline
								20, 37, // 50: resolution_gap_divergence = resolution_gap - resolution_gap_baseline
								32, 39, // 51: turnover_divergence = turnover - turnover_baseline
								32, 39, // 52: turnover_ratio = turnover / turnover_baseline
								33, 41, // 53: net_book_change_rate_divergence = net_book_change - net_book_change_baseline
								34, 43, // 54: signed_net_displayed_flow_rate_divergence = signed_net - signed_net_baseline
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
							),
						),
					),
					// Stage 6: Z-Scores (yields 55..59, total 60 values)
					data.NewValue[core.Primitive](
						data.NewSlice(0, 55),
						nomagique.NewNumber(
							data.NewSelect(
								49, 36, // 55: book_imbalance_zscore = book_imbalance_divergence / book_imbalance_scale
								50, 38, // 56: resolution_gap_zscore = resolution_gap_divergence / resolution_gap_scale
								51, 40, // 57: turnover_zscore = turnover_divergence / turnover_scale
								53, 42, // 58: net_book_change_rate_zscore = net_book_change_divergence / net_book_change_scale
								54, 44, // 59: signed_net_displayed_flow_rate_zscore = signed_net_divergence / signed_net_scale
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
							),
						),
					),
					// Stage 7: Select exact outputKeys (47 values)
					data.NewSelect(
						0,  //  0: book_notional:bid
						1,  //  1: book_notional:ask
						2,  //  2: book_notional
						0,  //  3: observed_notional:bid
						1,  //  4: observed_notional:ask
						2,  //  5: observed_notional
						3,  //  6: book_imbalance
						3,  //  7: observed_notional_imbalance
						7,  //  8: touch_imbalance
						20, //  9: imbalance_resolution_gap
						21, // 10: imbalance_resolution_distance
						8,  // 11: added_notional:bid
						9,  // 12: removed_notional:bid
						10, // 13: net_displayed_flow:bid
						12, // 14: added_notional:ask
						13, // 15: removed_notional:ask
						14, // 16: net_displayed_flow:ask
						31, // 17: flow_activity_imbalance
						35, // 18: book_imbalance_baseline
						49, // 19: book_imbalance_divergence
						55, // 20: book_imbalance_zscore
						37, // 21: resolution_gap_baseline
						50, // 22: resolution_gap_divergence
						56, // 23: resolution_gap_zscore
						45, // 24: book_imbalance_velocity
						46, // 25: resolution_gap_velocity
						25, // 26: added_notional_rate:bid
						26, // 27: added_notional_rate:ask
						27, // 28: removed_notional_rate:bid
						28, // 29: removed_notional_rate:ask
						29, // 30: net_displayed_flow_rate:bid
						30, // 31: net_displayed_flow_rate:ask
						32, // 32: book_turnover_rate
						33, // 33: net_book_change_rate
						34, // 34: signed_net_displayed_flow_rate
						39, // 35: turnover_baseline
						51, // 36: turnover_divergence
						57, // 37: turnover_zscore
						52, // 38: turnover_ratio
						41, // 39: net_book_change_rate_baseline
						53, // 40: net_book_change_rate_divergence
						58, // 41: net_book_change_rate_zscore
						43, // 42: signed_net_displayed_flow_rate_baseline
						54, // 43: signed_net_displayed_flow_rate_divergence
						59, // 44: signed_net_displayed_flow_rate_zscore
						47, // 45: historical_path_distance
						48, // 46: historical_path_percentile
					),
				),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", err))
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
			data.NewValue(
				pairs,
				atNano,
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
