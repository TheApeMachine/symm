package morphology

import (
	"context"
	"errors"
	"math"
	"sync"

	"github.com/theapemachine/errnie"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the book-morphology measuring instrument. It holds no logic of its
own: its entire behavior is one set of nomagique/distribution primitives per
symbol over a shared output map. The order-book levels are the only envelope
translation — folded into point streams the adapter cannot carry as maps —
then Wasserstein-1, Kolmogorov-Smirnov, concentration, and entropy score the
shape. Facts accumulate in the output map and are written once.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	books     broker.BookSource
	pipelines sync.Map
	metrics   [][4]string
}

type symbolPipeline struct {
	output   data.Map[float64]
	bids     [][2]float64
	asks     [][2]float64
	pairs    [2][][2]float64
	w1       core.Primitive
	ks       core.Primitive
	concBid  core.Primitive
	concAsk  core.Primitive
	entBid   core.Primitive
	entAsk   core.Primitive
	hasPrev  bool
	prevDist float64
}

/*
NewSignal composes the book-morphology instrument. The BookSource supplies the
aggregated levels whose notional shapes are measured.
*/
func NewSignal(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
		books: books,
		// {published label, output key, unit, timescale}
		metrics: [][4]string{
			{"book_shape_distance", "book_shape_distance", string(data.UnitDistance), string(data.TimescaleInstantaneous)},
			{"book_shape_ks", "book_shape_ks", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"concentration:bid", "concentration:bid", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"concentration:ask", "concentration:ask", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"entropy:bid", "entropy:bid", string(data.UnitNat), string(data.TimescaleInstantaneous)},
			{"entropy:ask", "entropy:ask", string(data.UnitNat), string(data.TimescaleInstantaneous)},
			{"morphology_change", "morphology_change", string(data.UnitDistance), string(data.TimescaleInstantaneous)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "morphology", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", err))
	}

	return signal
}

/*
Arena exposes the signal's ArenaOwner to the runtime Consumer.
*/
func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

func (signal *Signal) pipelineFor(symbol string) *symbolPipeline {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(*symbolPipeline)
	}

	pipe := &symbolPipeline{
		output:  data.NewOutputMap(),
		w1:      distribution.NewWasserstein1Pairs(),
		ks:      distribution.NewKolmogorovSmirnovPairs(),
		concBid: distribution.NewConcentrationPoints(),
		concAsk: distribution.NewConcentrationPoints(),
		entBid:  distribution.NewEntropyPoints(),
		entAsk:  distribution.NewEntropyPoints(),
	}

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipe)
	return actual.(*symbolPipeline)
}

/*
Step folds the shared book's levels into dimensionless distance-from-midpoint
point streams, drives the symbol's distribution primitives, and writes the
published morphology facts into a fresh Measurement allocated from the
signal's own arena. An absent or empty book yields no measurement: invalid
geometry is never fabricated into zero distance. A crossed or locked book is
corrupt state and halts the signal (broker.CrossedTouch). morphology_change is
omitted on a symbol's first observation.
*/
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

	pipe := signal.pipelineFor(prior.Label)

	clear(pipe.output.Values)
	pipe.bids = pipe.bids[:0]
	pipe.asks = pipe.asks[:0]

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

			pipe.bids = append(pipe.bids, [2]float64{(mid - price) / spread, price * qty})
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

			pipe.asks = append(pipe.asks, [2]float64{(price - mid) / spread, price * qty})
		}

		ok = len(pipe.bids) > 0 && len(pipe.asks) > 0
	})

	// Raised outside the book read lock. Book withholds pending and
	// checksum-diverging books, so a crossed touch is corrupt state: halt.
	if crossed {
		signal.Error(broker.CrossedTouch("morphology", prior.Label, bidPrice, askPrice))
		return nil
	}

	if !ok {
		return nil
	}

	pipe.pairs = [2][][2]float64{pipe.bids, pipe.asks}

	var distance, ks, concBid, concAsk, entBid, entAsk float64

	for pointer := range pipe.w1.Next(data.NewValue(pipe.pairs)) {
		distance = *(*float64)(pointer)
	}

	for pointer := range pipe.ks.Next(data.NewValue(pipe.pairs)) {
		ks = *(*float64)(pointer)
	}

	for pointer := range pipe.concBid.Next(data.NewValue(pipe.bids)) {
		concBid = *(*float64)(pointer)
	}

	for pointer := range pipe.concAsk.Next(data.NewValue(pipe.asks)) {
		concAsk = *(*float64)(pointer)
	}

	for pointer := range pipe.entBid.Next(data.NewValue(pipe.bids)) {
		entBid = *(*float64)(pointer)
	}

	for pointer := range pipe.entAsk.Next(data.NewValue(pipe.asks)) {
		entAsk = *(*float64)(pointer)
	}

	if err := errors.Join(
		pipe.w1.Error(), pipe.ks.Error(),
		pipe.concBid.Error(), pipe.concAsk.Error(),
		pipe.entBid.Error(), pipe.entAsk.Error(),
	); err != nil {
		signal.Error(err)
		return nil
	}

	if math.IsInf(distance, 0) || math.IsInf(ks, 0) {
		return nil
	}

	pipe.output.Values["book_shape_distance"] = distance
	pipe.output.Values["book_shape_ks"] = ks
	pipe.output.Values["concentration:bid"] = concBid
	pipe.output.Values["concentration:ask"] = concAsk
	pipe.output.Values["entropy:bid"] = entBid
	pipe.output.Values["entropy:ask"] = entAsk
	pipe.output.Values["morphology_change"] = 0

	if pipe.hasPrev {
		pipe.output.Values["morphology_change"] = math.Abs(distance - pipe.prevDist)
	}

	pipe.prevDist = distance
	pipe.hasPrev = true

	out := signal.arena.NewMeasurement(
		prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick, []*data.Measurement{prior},
	)
	out.Epoch = prior.Epoch
	out.Label = prior.Label
	out.Source = signal.Name()
	out.SeqIdx = prior.SeqIdx
	out.Tick = prior.Tick
	out.At = prior.At
	out.From = prior.At

	metrics := make([]*data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value := pipe.output.Values[metric[1]]

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	return out.Write(metrics...)
}
