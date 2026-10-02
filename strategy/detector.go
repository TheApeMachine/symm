package strategy

import (
	"math/rand/v2"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/lf"
)

/*
Detector scans epoch-length market tapes for a single symbol, identifies
excursions, calculates their PnL, splits them into precursor (A->B) and
holding (B->C) trajectories, and enqueues them for direct trie insertion.
*/
type Detector struct {
	queue *lf.Queue[[2][]*data.Measurement[float64]]
}

func NewDetector() *Detector {
	return &Detector{
		queue: lf.NewQueue[[2][]*data.Measurement[float64]](),
	}
}

/*
Scan performs a single-pass scan across an ordered slice of measurements for one symbol.
Measurements are WORM once written, so all emitted segments are sub-slices.
*/
func (detector *Detector) Scan(measurements []*data.Measurement[float64]) {
	if detector == nil || len(measurements) < 5 {
		return
	}

	lowIdx := -1
	highIdx := -1
	var lowPrice, highPrice float64

	for idx, measurement := range measurements {
		price, ok := quotePrice(measurement)
		if !ok {
			continue
		}

		if lowIdx == -1 {
			lowIdx = idx
			highIdx = idx
			lowPrice = price
			highPrice = price
			continue
		}

		if price < lowPrice {
			lowPrice = price
			lowIdx = idx
		}

		if price > highPrice {
			highPrice = price
			highIdx = idx
		}

		// Upward excursion: lowest point is before highest point
		if lowIdx < highIdx && highPrice > lowPrice {
			pullback := highPrice - price
			move := highPrice - lowPrice

			if pullback >= move*0.2 {
				detector.emitTrajectory(measurements, lowIdx, highIdx, lowPrice, highPrice)

				lowIdx = idx
				highIdx = idx
				lowPrice = price
				highPrice = price
			}
		}
	}

	if lowIdx >= 0 && highIdx > lowIdx && highPrice > lowPrice {
		detector.emitTrajectory(measurements, lowIdx, highIdx, lowPrice, highPrice)
	}
}

func (detector *Detector) Next() [2][]*data.Measurement[float64] {
	chunks, ok := detector.queue.Dequeue()

	if !ok {
		return [2][]*data.Measurement[float64]{}
	}

	return chunks
}

func (detector *Detector) emitTrajectory(
	measurements []*data.Measurement[float64],
	lowIdx, highIdx int,
	lowPrice, highPrice float64,
) {
	move := highPrice - lowPrice
	if move <= 0 || lowPrice <= 0 {
		return
	}

	// Exhaustion: exit safely before C upon deceleration, stagnation, or micro-pullback.
	exhaustIdx := highIdx
	prevPrice := lowPrice
	maxVelocity := 0.0

	for i := lowIdx + 1; i <= highIdx; i++ {
		p, ok := quotePrice(measurements[i])
		if !ok {
			continue
		}

		delta := p - prevPrice
		if delta > maxVelocity {
			maxVelocity = delta
		}

		progress := (p - lowPrice) / move

		// The faster the momentum, the more paranoid: exit upon slowdown once profitable.
		if progress >= 0.50 {
			if delta <= 0 || (maxVelocity > 0 && delta < maxVelocity*0.5) {
				exhaustIdx = i
				break
			}
		}

		prevPrice = p
	}

	// Randomized Point A before Point B
	startA := 0
	if lowIdx > 2 {
		startA = rand.IntN(lowIdx - 1)
	}

	precursor := measurements[startA:lowIdx]
	var holding []*data.Measurement[float64]
	if exhaustIdx > lowIdx+1 {
		holding = measurements[lowIdx+1 : exhaustIdx+1]
	}

	detector.queue.Enqueue([2][]*data.Measurement[float64]{
		precursor,
		holding,
	})
}

func quotePrice(measurement *data.Measurement[float64]) (float64, bool) {
	if measurement == nil {
		return 0, false
	}

	if p, ok := measurement.LookupMetric("price"); ok && p.Raw > 0 {
		return p.Raw, true
	}

	bid, hasBid := measurement.LookupMetric("bid")
	ask, hasAsk := measurement.LookupMetric("ask")

	if hasBid && hasAsk && bid.Raw > 0 && ask.Raw > 0 {
		return bid.Exact.Add(ask.Exact).Div(decimal.NewFromInt64(2)).Float64(), true
	}

	return 0, false
}
