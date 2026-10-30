package morphology

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

var (
	errShape  = errors.New("morphology: book too short to score")
	errDomain = errors.New("morphology: book is crossed or non-positive")
)

type point struct {
	pos    float64
	weight float64
}

type symbolState struct {
	hasPrev  bool
	prevBook []point
}

type Signal struct {
	*runtime.System
	books  *broker.Book
	states *sync.Map
	keys   []string
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books:  books,
		states: &sync.Map{},
		keys: []string{
			"book_shape_stable",
		},
	}

	signal.System = runtime.NewSystem(ctx, "morphology", signal)
	return signal
}

/*
Step processes the book of every distinct symbol traded in the tick's frame;
each symbol writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	symbols := parent.DistinctSymbols(trades)

	if err := parent.RunSymbols(signal.Context(), symbols, func(symbol string) {
		signal.step(value, symbol)
	}); err != nil {
		signal.Error(err)
	}

	return value
}

func (signal *Signal) step(value *data.Value, symbol string) *data.Value {
	if signal.Status() != runtime.READY {
		return value
	}

	var bookAvailable bool
	var bids []float64
	var asks []float64

	signal.books.Book(symbol, func(b *spotbook.Book) {
		bookAvailable = true

		bestBid := b.BestBid()
		bestAsk := b.BestAsk()
		if bestBid == nil || bestAsk == nil || bestBid.Price == nil || bestAsk.Price == nil {
			return
		}

		bestBidPrice := bestBid.Price.Float64()
		bestAskPrice := bestAsk.Price.Float64()

		// Calculate expects both sides prefixed with [bestBid, bestAsk] prices.
		bids = []float64{bestBidPrice, bestAskPrice}
		asks = []float64{bestBidPrice, bestAskPrice}

		for _, level := range b.Bids.Levels {
			bids = append(bids, level.Price.Float64(), level.Quantity.Float64())
		}

		for _, level := range b.Asks.Levels {
			asks = append(asks, level.Price.Float64(), level.Quantity.Float64())
		}
	})

	if !bookAvailable || len(bids) < 2 || len(asks) < 2 {
		return value
	}

	res, err := signal.Calculate(symbol, bids, asks)
	if err != nil || len(res) == 0 {
		return value
	}

	stable := bookShapeStable(res)

	keys := []string{
		signal.Name(),
		symbol,
	}

	value.Append([]float64{stable}, append(keys, signal.keys...)...)

	audit.Record.Write(map[string]any{
		"signal": signal.Name(),
		"symbol": symbol,
		"stable": stable,
	})

	return value
}

/*
bookShapeStable is high when the normalized depth curve is barely morphing
between snapshots — ladder shape holding steady under the move.
*/
func bookShapeStable(res []float64) float64 {
	if len(res) < 7 {
		return 0
	}

	morphChange := res[6]
	if morphChange <= 0 {
		return 1
	}

	return 1 / (1 + morphChange)
}

func (signal *Signal) Calculate(stateKey string, bids, asks []float64) ([]float64, error) {
	if len(bids) < 2 || len(asks) < 2 {
		return nil, errShape
	}

	bestBidPrice := bids[0]
	bestAskPrice := bids[1]

	if bestAskPrice <= bestBidPrice {
		return nil, errDomain
	}

	spread := bestAskPrice - bestBidPrice
	midPrice := (bestBidPrice + bestAskPrice) * 0.5

	numBids := (len(bids) - 2) / 2
	bidPoints := make([]point, 0, numBids)
	var totalBidNotional float64

	for index := 2; index < len(bids); index += 2 {
		price := bids[index]
		qty := bids[index+1]
		if price <= 0 || qty < 0 {
			return nil, errDomain
		}
		coord := (price - midPrice) / spread
		notional := price * qty
		totalBidNotional += notional
		bidPoints = append(bidPoints, point{pos: coord, weight: notional})
	}

	numAsks := (len(asks) - 2) / 2
	askPoints := make([]point, 0, numAsks)
	var totalAskNotional float64

	for index := 2; index < len(asks); index += 2 {
		price := asks[index]
		qty := asks[index+1]
		if price <= 0 || qty < 0 {
			return nil, errDomain
		}
		coord := (price - midPrice) / spread
		notional := price * qty
		totalAskNotional += notional
		askPoints = append(askPoints, point{pos: coord, weight: notional})
	}

	if totalBidNotional == 0 || totalAskNotional == 0 {
		return nil, nil
	}

	sort.SliceStable(bidPoints, func(left, right int) bool {
		return bidPoints[left].pos < bidPoints[right].pos
	})

	sort.SliceStable(askPoints, func(left, right int) bool {
		return askPoints[left].pos < askPoints[right].pos
	})

	var concBid, entropyBid float64
	for index := range bidPoints {
		bidPoints[index].weight /= totalBidNotional
		weight := bidPoints[index].weight
		concBid += weight * weight
		if weight > 0 {
			entropyBid -= weight * math.Log(weight)
		}
	}

	var concAsk, entropyAsk float64
	for index := range askPoints {
		askPoints[index].weight /= totalAskNotional
		weight := askPoints[index].weight
		concAsk += weight * weight
		if weight > 0 {
			entropyAsk -= weight * math.Log(weight)
		}
	}

	foldedBids := make([]point, len(bidPoints))
	for index, pt := range bidPoints {
		foldedBids[index] = point{pos: math.Abs(pt.pos), weight: pt.weight}
	}

	sort.SliceStable(foldedBids, func(left, right int) bool {
		return foldedBids[left].pos < foldedBids[right].pos
	})

	ks, shapeDist := mergedWalk(foldedBids, askPoints)

	currentBook := make([]point, 0, len(bidPoints)+len(askPoints))
	for _, pt := range bidPoints {
		currentBook = append(currentBook, point{pos: pt.pos, weight: pt.weight * 0.5})
	}
	for _, pt := range askPoints {
		currentBook = append(currentBook, point{pos: pt.pos, weight: pt.weight * 0.5})
	}

	untypedState, _ := signal.states.LoadOrStore(stateKey, &symbolState{})
	state := untypedState.(*symbolState)

	var morphChange float64
	hasMorphChange := state.hasPrev
	if hasMorphChange {
		_, morphChange = mergedWalk(state.prevBook, currentBook)
	}

	state.hasPrev = true
	state.prevBook = currentBook

	res := []float64{shapeDist, ks, concBid, concAsk, entropyBid, entropyAsk}
	if hasMorphChange {
		res = append(res, morphChange)
	}

	return res, nil
}

func mergedWalk(streamA, streamB []point) (float64, float64) {
	if len(streamA) == 0 || len(streamB) == 0 {
		return 0.0, 0.0
	}

	var totalA, totalB float64
	for _, pt := range streamA {
		totalA += pt.weight
	}
	for _, pt := range streamB {
		totalB += pt.weight
	}

	if totalA == 0 || totalB == 0 {
		return 0.0, 0.0
	}

	var idxA, idxB int
	var cumA, cumB float64
	var prevPos, ks, distance float64
	first := true

	for idxA < len(streamA) || idxB < len(streamB) {
		pos := 0.0
		if idxA < len(streamA) && idxB < len(streamB) {
			pos = streamA[idxA].pos
			if streamB[idxB].pos < pos {
				pos = streamB[idxB].pos
			}
		}
		if idxA < len(streamA) && idxB >= len(streamB) {
			pos = streamA[idxA].pos
		}
		if idxB < len(streamB) && idxA >= len(streamA) {
			pos = streamB[idxB].pos
		}

		if !first {
			distance += math.Abs(cumA-cumB) * (pos - prevPos)
		}
		first = false

		for idxA < len(streamA) && streamA[idxA].pos == pos {
			cumA += streamA[idxA].weight / totalA
			idxA++
		}

		for idxB < len(streamB) && streamB[idxB].pos == pos {
			cumB += streamB[idxB].weight / totalB
			idxB++
		}

		diff := math.Abs(cumA - cumB)
		if diff > ks {
			ks = diff
		}
		prevPos = pos
	}

	return ks, distance
}
