package depthflow

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
	pipeline *nomagique.Number
	history  sync.Map
}

type symbolHistory struct {
	prevBids     map[float64]float64
	prevAsks     map[float64]float64
	prevNotional float64
	prevAt       time.Time
	hasPrev      bool
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					data.NewValue[core.Primitive](
						nomagique.NewNumber(
							data.NewSlice(0, 10),
							transport.NewParallel(
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewAdd()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewSubtract()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewAdd()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewSubtract()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewSubtract()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewSubtract()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewDivide()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewDivide()),
								nomagique.NewNumber(transport.NewSpread[float64](), arithmetic.NewSubtract()),
								nomagique.NewNumber(transport.NewSpread[float64](), calculus.NewAbsolute()),
							),
						),
						data.NewSlice(10, 20),
					),
					data.NewValue[core.Primitive](
						data.NewSlice(0, 20),
						nomagique.NewNumber(
							data.NewSelect(
								0, 0,
								1, 1,
								2, 2,
								3, 3,
								4, 4,
								5, 5,
								6, 6,
								7, 7,
								8, 8,
								9, 9,
								10, 10,
								11, 11,
								12, 12,
								13, 13,
							),
							data.NewBatch(2, 2),
							transport.NewParallel(
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), adaptive.NewBaseline(adaptive.NewWindow())),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), temporal.NewVelocity()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), data.NewSlice(0, 1), transport.NewPass()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewSubtract()),
								nomagique.NewNumber(data.NewUnpack(), arithmetic.NewDivide()),
							),
						),
					),
					data.NewSelect(
						0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
						10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
						20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
						30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
						40, 41, 42, 43, 44, 45, 46,
					),
				),
				data.NewMessage(data.WRITE, "symbolstore", "depthflow_state", data.NewValue[core.Primitive]()),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", err))
	}

	return signal
}

func (signal *Signal) getHistory(symbol string) *symbolHistory {
	val, ok := signal.history.Load(symbol)
	if ok {
		return val.(*symbolHistory)
	}

	hist := &symbolHistory{
		prevBids: make(map[float64]float64),
		prevAsks: make(map[float64]float64),
	}
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
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", nil))
		return nil
	}

	hist := signal.getHistory(prior.Label)

	currBids := make(map[float64]float64)
	currAsks := make(map[float64]float64)

	var bidNotional, askNotional, touchBid, touchAsk float64
	var addedBid, removedBid, addedAsk, removedAsk float64
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

		if kraken.Float64(ask.Price) <= kraken.Float64(bid.Price) {
			crossed = true
			crossedBid = kraken.Float64(bid.Price)
			crossedAsk = kraken.Float64(ask.Price)
			return
		}

		for cursor, count := bid, 0; cursor != nil && count < 100; cursor, count = cursor.Lower, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 || math.IsNaN(price) || math.IsNaN(qty) || math.IsInf(price, 0) || math.IsInf(qty, 0) {
				continue
			}

			if len(currBids) == 0 {
				touchBid = price * qty
			}

			currBids[price] = qty
			bidNotional += price * qty
		}

		for cursor, count := ask, 0; cursor != nil && count < 100; cursor, count = cursor.Higher, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 || math.IsNaN(price) || math.IsNaN(qty) || math.IsInf(price, 0) || math.IsInf(qty, 0) {
				continue
			}

			if len(currAsks) == 0 {
				touchAsk = price * qty
			}

			currAsks[price] = qty
			askNotional += price * qty
		}

		ok = len(currBids) > 0 && len(currAsks) > 0
	})

	if crossed {
		signal.Error(broker.CrossedTouch("depthflow", prior.Label, crossedBid, crossedAsk))
		return nil
	}

	if !ok {
		return nil
	}

	hadPrev := hist.hasPrev
	prevAt := hist.prevAt
	flowReady := hadPrev && prior.At.After(prevAt)

	if hadPrev {
		for price, qty := range currBids {
			delta := price * (qty - hist.prevBids[price])
			addedBid += math.Max(delta, 0)
			removedBid += math.Max(-delta, 0)
		}

		for price, qty := range hist.prevBids {
			if _, held := currBids[price]; !held {
				removedBid += price * qty
			}
		}

		for price, qty := range currAsks {
			delta := price * (qty - hist.prevAsks[price])
			addedAsk += math.Max(delta, 0)
			removedAsk += math.Max(-delta, 0)
		}

		for price, qty := range hist.prevAsks {
			if _, held := currAsks[price]; !held {
				removedAsk += price * qty
			}
		}
	}

	totalNotional := bidNotional + askNotional
	touchTotal := touchBid + touchAsk
	var bookImbalance, touchImbalance, imbalanceGap, imbalanceDistance float64
	if totalNotional > 0 {
		bookImbalance = (bidNotional - askNotional) / totalNotional
	}
	if touchTotal > 0 {
		touchImbalance = (touchBid - touchAsk) / touchTotal
	}
	imbalanceGap = touchImbalance - bookImbalance
	imbalanceDistance = math.Abs(imbalanceGap)

	netDisplayedFlowBid := addedBid - removedBid
	netDisplayedFlowAsk := addedAsk - removedAsk
	signedNetFlow := netDisplayedFlowBid - netDisplayedFlowAsk
	grossDisplayedFlow := math.Abs(netDisplayedFlowBid) + math.Abs(netDisplayedFlowAsk)
	var flowActivityImbalance float64
	if grossDisplayedFlow > 0 {
		flowActivityImbalance = signedNetFlow / grossDisplayedFlow
	}

	var addedRateBid, addedRateAsk, removedRateBid, removedRateAsk float64
	var netFlowRateBid, netFlowRateAsk, bookTurnoverRate, netBookChangeRate, signedNetDisplayedFlowRate float64

	if flowReady {
		elapsed := prior.At.Sub(prevAt).Seconds()
		if elapsed > 0 {
			addedRateBid = addedBid / elapsed
			addedRateAsk = addedAsk / elapsed
			removedRateBid = removedBid / elapsed
			removedRateAsk = removedAsk / elapsed
			netFlowRateBid = netDisplayedFlowBid / elapsed
			netFlowRateAsk = netDisplayedFlowAsk / elapsed

			refNotional := (totalNotional + hist.prevNotional) / 2.0
			refExposure := refNotional * elapsed
			if refExposure > 0 {
				bookActivity := (addedBid + removedBid) + (addedAsk + removedAsk)
				bookTurnoverRate = bookActivity / refExposure
				netBookChangeRate = (totalNotional - hist.prevNotional) / refExposure
				signedNetDisplayedFlowRate = signedNetFlow / refExposure
			}
		}
	}

	output := make(map[string]float64)

	rawMetrics := []float64{
		bidNotional,
		askNotional,
		totalNotional,
		bidNotional,
		askNotional,
		totalNotional,
		bookImbalance,
		bookImbalance,
		touchImbalance,
		imbalanceGap,
		imbalanceDistance,
		addedBid,
		removedBid,
		netDisplayedFlowBid,
		addedAsk,
		removedAsk,
		netDisplayedFlowAsk,
		flowActivityImbalance,
		0, 0, 0, // book_imbalance baseline, divergence, zscore
		0, 0, 0, // resolution_gap baseline, divergence, zscore
		0, 0, // velocities
		addedRateBid,
		addedRateAsk,
		removedRateBid,
		removedRateAsk,
		netFlowRateBid,
		netFlowRateAsk,
		bookTurnoverRate,
		netBookChangeRate,
		signedNetDisplayedFlowRate,
		0, 0, 0, 0, // turnover baseline, divergence, zscore, ratio
		0, 0, 0, // net_book_change baseline, divergence, zscore
		0, 0, 0, // signed_net baseline, divergence, zscore
		0, 0, // historical path distance, percentile
	}

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

	hist.prevBids = currBids
	hist.prevAsks = currAsks
	hist.prevNotional = totalNotional
	hist.prevAt = prior.At
	hist.hasPrev = true

	out := prior.Next(signal.Name(), output)
	if flowReady {
		out.From = prevAt
	}

	return out
}
