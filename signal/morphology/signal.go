package morphology

import (
	"context"
	"math"
	"sync"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

var outputKeys = []string{
	"book_shape_distance",
	"book_shape_ks",
	"concentration:bid",
	"concentration:ask",
	"entropy:bid",
	"entropy:ask",
	"morphology_change",
}

type Signal struct {
	*runtime.System
	books    broker.BookSource
	pipeline *nomagique.Number
	history  sync.Map
}

type symbolHistory struct {
	hasPrev  bool
	prevDist float64
	w1       core.Primitive
	ks       core.Primitive
	concBid  core.Primitive
	concAsk  core.Primitive
	entBid   core.Primitive
	entAsk   core.Primitive
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					data.NewSelect(0, 1, 2, 3, 4, 5, 6),
				),
				data.NewMessage(data.WRITE, "symbolstore", "morphology_state", data.NewValue[core.Primitive]()),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "morphology", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", err))
	}

	return signal
}

func (signal *Signal) getHistory(symbol string) *symbolHistory {
	val, ok := signal.history.Load(symbol)
	if ok {
		return val.(*symbolHistory)
	}

	hist := &symbolHistory{
		w1:      distribution.NewWasserstein1Pairs(),
		ks:      distribution.NewKolmogorovSmirnovPairs(),
		concBid: distribution.NewConcentrationPoints(),
		concAsk: distribution.NewConcentrationPoints(),
		entBid:  distribution.NewEntropyPoints(),
		entAsk:  distribution.NewEntropyPoints(),
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
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", nil))
		return nil
	}

	var bids, asks [][2]float64
	var bidPrice, askPrice float64
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

		bidPrice = kraken.Float64(bid.Price)
		askPrice = kraken.Float64(ask.Price)
		spread := askPrice - bidPrice

		if spread <= 0 && !math.IsNaN(spread) {
			crossed = true
			return
		}

		if math.IsNaN(spread) || math.IsInf(spread, 0) {
			return
		}

		mid := (bidPrice + askPrice) / 2.0

		for cursor, count := bid, 0; cursor != nil && count < 100; cursor, count = cursor.Lower, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 || math.IsNaN(price) || math.IsNaN(qty) {
				continue
			}

			bids = append(bids, [2]float64{(mid - price) / spread, price * qty})
		}

		for cursor, count := ask, 0; cursor != nil && count < 100; cursor, count = cursor.Higher, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 || math.IsNaN(price) || math.IsNaN(qty) {
				continue
			}

			asks = append(asks, [2]float64{(price - mid) / spread, price * qty})
		}

		ok = len(bids) > 0 && len(asks) > 0
	})

	if crossed {
		signal.Error(broker.CrossedTouch("morphology", prior.Label, bidPrice, askPrice))
		return nil
	}

	if !ok {
		return nil
	}

	hist := signal.getHistory(prior.Label)
	pairs := [2][][2]float64{bids, asks}

	var distance, ks, concBid, concAsk, entBid, entAsk float64

	for pointer := range hist.w1.Next(data.NewValue(unsafe.Pointer(&pairs)).Next(nil)) {
		distance = *(*float64)(pointer)
	}

	for pointer := range hist.ks.Next(data.NewValue(unsafe.Pointer(&pairs)).Next(nil)) {
		ks = *(*float64)(pointer)
	}

	for pointer := range hist.concBid.Next(data.NewValue(unsafe.Pointer(&bids)).Next(nil)) {
		concBid = *(*float64)(pointer)
	}

	for pointer := range hist.concAsk.Next(data.NewValue(unsafe.Pointer(&asks)).Next(nil)) {
		concAsk = *(*float64)(pointer)
	}

	for pointer := range hist.entBid.Next(data.NewValue(unsafe.Pointer(&bids)).Next(nil)) {
		entBid = *(*float64)(pointer)
	}

	for pointer := range hist.entAsk.Next(data.NewValue(unsafe.Pointer(&asks)).Next(nil)) {
		entAsk = *(*float64)(pointer)
	}

	if math.IsInf(distance, 0) || math.IsInf(ks, 0) {
		return nil
	}

	var morphChange float64
	if hist.hasPrev {
		morphChange = math.Abs(distance - hist.prevDist)
	}
	hist.prevDist = distance
	hist.hasPrev = true

	rawMetrics := []float64{
		distance,
		ks,
		concBid,
		concAsk,
		entBid,
		entAsk,
		morphChange,
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

	return prior.Next(signal.Name(), output)
}
