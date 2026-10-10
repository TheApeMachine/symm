package morphology

import (
	"context"
	"math"
	"sort"
	"strconv"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
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
	books  broker.BookSource
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "morphology", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", nil))
		return nil
	}

	var bids, asks []float64
	var found bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
			return
		}

		bestBidPrice := book.BestBid().Price.Float64()
		bestAskPrice := book.BestAsk().Price.Float64()
		found = true

		bids = append(bids, bestBidPrice, bestAskPrice)
		asks = append(asks, bestBidPrice, bestAskPrice)

		for level := book.BestBid(); level != nil; level = level.Lower {
			bids = append(bids, level.Price.Float64(), level.Quantity.Float64())
		}

		for level := book.BestAsk(); level != nil; level = level.Higher {
			asks = append(asks, level.Price.Float64(), level.Quantity.Float64())
		}
	})

	if !found || len(bids) <= 2 || len(asks) <= 2 {
		return nil
	}

	stateKey := strconv.FormatInt(prior.Epoch, 10) + "/" + prior.Label
	res, err := signal.Calculate(stateKey, bids, asks)
	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] "+prior.Label+": calculation failed", err))
		return nil
	}

	if len(res) == 0 {
		return nil
	}

	out := map[string]float64{
		"book_shape_distance": res[0],
		"book_shape_ks":       res[1],
		"concentration:bid":   res[2],
		"concentration:ask":   res[3],
		"entropy:bid":         res[4],
		"entropy:ask":         res[5],
	}

	// Calculate appends the change only once a prior shape exists.
	if len(res) > 6 {
		out["morphology_change"] = res[6]
	}

	return prior.Next(signal.Name(), out)
}

func (signal *Signal) Calculate(stateKey string, bids, asks []float64) ([]float64, error) {
	if len(bids) < 2 || len(asks) < 2 {
		return nil, core.ErrShape
	}

	bestBidPrice := bids[0]
	bestAskPrice := bids[1]

	if bestAskPrice <= bestBidPrice {
		return nil, core.ErrDomain
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
			return nil, core.ErrDomain
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
			return nil, core.ErrDomain
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

	signal.mu.Lock()
	state, exists := signal.states[stateKey]
	if !exists {
		state = &symbolState{}
		signal.states[stateKey] = state
	}

	var morphChange float64
	hasMorphChange := state.hasPrev
	if hasMorphChange {
		_, morphChange = mergedWalk(state.prevBook, currentBook)
	}

	state.hasPrev = true
	state.prevBook = currentBook
	signal.mu.Unlock()

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
