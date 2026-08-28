/*
Package morphology measures the shape of an order book as a geometric object:
where displayed notional sits along the price axis, how one-sided the two
shapes are, how concentrated each side is, and how much the whole shape moved
since the last observation.

It is deliberately a measuring instrument, never a judge. Every emitted fact
is a dimensionless, unitless description of book geometry: a distance in spread
units, a cumulative-disagreement statistic, a concentration, an entropy, and a
structural change. There is no manipulation score, no "synthetic"/"suspicious"
label, no fixed symmetry threshold, and no arbitrary depth bucket — the shape
is the book's own levels, normalized by its own current spread and each level
weighted by its own side notional.

The generic distribution mathematics (Wasserstein, Kolmogorov-Smirnov,
entropy, Herfindahl) lives in nomagique/distribution; this package only
projects a live book into shape coordinates and calls it.
*/
package morphology

import (
	"sort"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"

	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Output slots the projector names into Measurement metrics.
*/
var (
	symbolShapeDistance    = nmtypes.MustIntern("morphology/book_shape_distance")
	symbolShapeKS          = nmtypes.MustIntern("morphology/book_shape_ks")
	symbolConcentrationBid = nmtypes.MustIntern("morphology/concentration_bid")
	symbolConcentrationAsk = nmtypes.MustIntern("morphology/concentration_ask")
	symbolEntropyBid       = nmtypes.MustIntern("morphology/entropy_bid")
	symbolEntropyAsk       = nmtypes.MustIntern("morphology/entropy_ask")
	symbolMorphologyChange = nmtypes.MustIntern("morphology/morphology_change")
)

// shape is one normalized book-shape: sorted positions in spread units paired
// with the normalized notional weight at each position.
type shape struct {
	positions []float64
	weights   []float64
}

/*
Book is the book-shape market entity. It reads the shared book per step and
projects one dimensionless shape Measurement. It retains the prior whole-book
shape per symbol — a single overwritten shape each, the same bounded-resident-
state contract as the shared book — so structural change is measured causally.
*/
type Book struct {
	workspace *runtime.Workspace
	projector *data.Projector

	mu       sync.Mutex
	previous map[string]shape
}

func NewBook(workspace *runtime.Workspace) *Book {
	return &Book{
		workspace: workspace,
		previous:  make(map[string]shape),
		projector: data.NewProjector(
			data.Binding{From: symbolShapeDistance, Name: "book_shape_distance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolShapeKS, Name: "book_shape_ks", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolConcentrationBid, Name: "concentration:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolConcentrationAsk, Name: "concentration:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolEntropyBid, Name: "entropy:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolEntropyAsk, Name: "entropy:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMorphologyChange, Name: "morphology_change", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

func (morphology *Book) Close() error { return nil }

/*
Step reads the shared book for one symbol, projects it into spread-normalized
shape coordinates, and emits exactly one descriptive Measurement. A missing or
degenerate book (crossed, no spread, one empty side) yields no measurement; the
caller skips it rather than panicking.
*/
func (morphology *Book) Step(symbol string, at time.Time) *data.Measurement[float64] {
	if morphology == nil || morphology.workspace == nil {
		return nil
	}

	orderBook := morphology.readBook(symbol)

	if orderBook == nil {
		return nil
	}

	bidPositions, bidWeights, askPositions, askWeights, ok := shapeCoordinates(orderBook)

	if !ok {
		return nil
	}

	bidWeights, _ = distribution.Normalize(bidWeights)
	askWeights, _ = distribution.Normalize(askWeights)

	unionPositions, unionBidWeights, unionAskWeights := sharedSupport(bidPositions, bidWeights, askPositions, askWeights)

	shapeDistance := distribution.Wasserstein1(unionPositions, unionBidWeights, unionAskWeights)
	shapeKS := distribution.KolmogorovSmirnov(unionPositions, unionBidWeights, unionAskWeights)
	concentrationBid := distribution.Concentration(bidWeights)
	concentrationAsk := distribution.Concentration(askWeights)
	entropyBid := distribution.Entropy(bidWeights)
	entropyAsk := distribution.Entropy(askWeights)

	current := wholeBookShape(unionPositions, unionBidWeights, unionAskWeights)

	morphologyChange, changed := morphology.recordChange(symbol, current)

	input := nmtypes.Frame{}
	input.Put(symbolShapeDistance, shapeDistance)
	input.Put(symbolShapeKS, shapeKS)
	input.Put(symbolConcentrationBid, concentrationBid)
	input.Put(symbolConcentrationAsk, concentrationAsk)
	input.Put(symbolEntropyBid, entropyBid)
	input.Put(symbolEntropyAsk, entropyAsk)

	if changed {
		input.Put(symbolMorphologyChange, morphologyChange)
	}

	return morphology.projector.Project(symbol, "morphology", at, at, input)
}

/*
readBook returns the live aggregated book for one symbol from the workspace
shared pool, matching the depthflow signal's access pattern.
*/
func (morphology *Book) readBook(symbol string) *book.Book {
	if shared, found := morphology.workspace.Shared("api", ""); found && shared != nil {
		if api, isAPI := shared.(*websocket.API); isAPI && api != nil {
			var orderBook *book.Book

			api.Book(symbol, func(sharedBook *book.Book) {
				orderBook = sharedBook
			})

			return orderBook
		}
	}

	if sharedBook, found := morphology.workspace.Shared("book", symbol); found && sharedBook != nil {
		if currentBook, isBook := sharedBook.(*book.Book); isBook {
			return currentBook
		}
	}

	return nil
}

/*
recordChange stores the current whole-book shape and returns how far it moved
from the previously retained shape of the same symbol, measured as the
Wasserstein-1 distance between the two normalized whole shapes on their shared
support. The first observation of a symbol has no prior and reports no change.
*/
func (morphology *Book) recordChange(symbol string, current shape) (float64, bool) {
	morphology.mu.Lock()
	previous, hadPrevious := morphology.previous[symbol]
	morphology.previous[symbol] = current
	morphology.mu.Unlock()

	if !hadPrevious {
		return 0, false
	}

	union, previousWeights, currentWeights := sharedSupport(previous.positions, previous.weights, current.positions, current.weights)

	return distribution.Wasserstein1(union, previousWeights, currentWeights), true
}

/*
shapeCoordinates projects a book's levels into spread-normalized coordinates:
each level's position is (price − midpoint) / spread (so the bid touch sits at
−0.5 and the ask touch at +0.5 in spread units), and each level's weight is its
displayed notional (price × quantity). ok is false when the book is degenerate
— missing touch, non-positive spread, or an empty side — so no distance is
fabricated on a book with no shape.
*/
func shapeCoordinates(orderBook *book.Book) ([]float64, []float64, []float64, []float64, bool) {
	bestBid := orderBook.BestBid()
	bestAsk := orderBook.BestAsk()

	if bestBid == nil || bestAsk == nil || bestBid.Price == nil || bestAsk.Price == nil {
		return nil, nil, nil, nil, false
	}

	bidPrice := bestBid.Price.Float64()
	askPrice := bestAsk.Price.Float64()
	spread := askPrice - bidPrice

	if spread <= 0 {
		return nil, nil, nil, nil, false
	}

	midpoint := (bidPrice + askPrice) / 2

	bidPositions := make([]float64, 0)
	bidWeights := make([]float64, 0)
	askPositions := make([]float64, 0)
	askWeights := make([]float64, 0)

	if orderBook.Bids != nil {
		for _, level := range orderBook.Bids.Levels {
			if level == nil || level.Price == nil || level.Quantity == nil {
				continue
			}

			weight := level.Price.Float64() * level.Quantity.Float64()

			if weight <= 0 {
				continue
			}

			bidPositions = append(bidPositions, (level.Price.Float64()-midpoint)/spread)
			bidWeights = append(bidWeights, weight)
		}
	}

	if orderBook.Asks != nil {
		for _, level := range orderBook.Asks.Levels {
			if level == nil || level.Price == nil || level.Quantity == nil {
				continue
			}

			weight := level.Price.Float64() * level.Quantity.Float64()

			if weight <= 0 {
				continue
			}

			askPositions = append(askPositions, (level.Price.Float64()-midpoint)/spread)
			askWeights = append(askWeights, weight)
		}
	}

	if len(bidPositions) == 0 || len(askPositions) == 0 {
		return nil, nil, nil, nil, false
	}

	return bidPositions, bidWeights, askPositions, askWeights, true
}

/*
sharedSupport places two distributions onto their sorted union of positions.
For each union position the returned weight for side A is A's own weight there
(or zero if the position belongs only to B), and symmetrically for B. The union
is ascending and both weight slices are zero-padded so a position present on
only one side is represented as missing mass on the other.
*/
func sharedSupport(positionsA, weightsA, positionsB, weightsB []float64) ([]float64, []float64, []float64) {
	weightByA := make(map[float64]float64, len(positionsA))
	weightByB := make(map[float64]float64, len(positionsB))
	seen := make(map[float64]bool, len(positionsA)+len(positionsB))

	for index, position := range positionsA {
		weightByA[position] = weightsA[index]
		seen[position] = true
	}

	for index, position := range positionsB {
		weightByB[position] = weightsB[index]
		seen[position] = true
	}

	union := make([]float64, 0, len(seen))

	for position := range seen {
		union = append(union, position)
	}

	sort.Float64s(union)

	unionWeightsA := make([]float64, len(union))
	unionWeightsB := make([]float64, len(union))

	for index, position := range union {
		unionWeightsA[index] = weightByA[position]
		unionWeightsB[index] = weightByB[position]
	}

	return union, unionWeightsA, unionWeightsB
}

/*
wholeBookShape folds both sides into one combined mass profile along the price
axis for the structural-change comparison: the union positions with each
side's normalized weight halved, so the two sides together still sum to 1.
*/
func wholeBookShape(unionPositions, unionBidWeights, unionAskWeights []float64) shape {
	weights := make([]float64, len(unionPositions))

	for index := range unionPositions {
		weights[index] = (unionBidWeights[index] + unionAskWeights[index]) / 2
	}

	return shape{
		positions: append([]float64(nil), unionPositions...),
		weights:   weights,
	}
}
