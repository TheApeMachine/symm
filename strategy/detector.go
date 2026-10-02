package strategy

import (
	"math/rand/v2"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/lf"
)

/*
Trajectory contains the aligned market observations for one detected excursion:
its entry and exit prices, the aligned precursor and holding measurements from
signals and logic stages, and the raw ticker slices for trajectory telemetry.
*/
type Trajectory struct {
	Symbol    string
	EntryAsk  *decimal.Decimal
	ExitBid   *decimal.Decimal
	Precursor []*data.Measurement[float64]
	Holding   []*data.Measurement[float64]
	Ticks     [2][]*data.Measurement[float64]
}

/*
Detector scans epoch-length market tapes for a single symbol, identifies
excursions, aligns signals and logic stages with ticker slices, and enqueues
complete trajectories for model reinforcement.
*/
type Detector struct {
	queue *lf.Queue[Trajectory]
}

func NewDetector() *Detector {
	return &Detector{
		queue: lf.NewQueue[Trajectory](),
	}
}

func (detector *Detector) Next() (Trajectory, bool) {
	if detector == nil || detector.queue == nil {
		return Trajectory{}, false
	}

	trajectory, ok := detector.queue.Dequeue()

	if !ok {
		return Trajectory{}, false
	}

	return trajectory, true
}

/*
Scan performs a single-pass scan across an ordered slice of measurements for one symbol.
Measurements are WORM once written, so all emitted segments are sub-slices.
*/
func (detector *Detector) Scan(measurements []*data.Measurement[float64]) {
	if detector == nil || len(measurements) < 5 {
		return
	}

	lowIndex := -1
	highIndex := -1
	var lowPrice, highPrice float64

	for index, measurement := range measurements {
		price, ok := quotePrice(measurement)

		if !ok {
			continue
		}

		if lowIndex == -1 {
			lowIndex = index
			highIndex = index
			lowPrice = price
			highPrice = price
			continue
		}

		if price < lowPrice {
			lowPrice = price
			lowIndex = index
		}

		if price > highPrice {
			highPrice = price
			highIndex = index
		}

		// Upward excursion: lowest point is before highest point
		if lowIndex < highIndex && highPrice > lowPrice {
			pullback := highPrice - price
			move := highPrice - lowPrice

			if pullback >= move*0.2 {
				detector.emitTrajectory(measurements, lowIndex, highIndex, lowPrice, highPrice)

				lowIndex = index
				highIndex = index
				lowPrice = price
				highPrice = price
			}
		}
	}

	if lowIndex >= 0 && highIndex > lowIndex && highPrice > lowPrice {
		detector.emitTrajectory(measurements, lowIndex, highIndex, lowPrice, highPrice)
	}
}

func (detector *Detector) emitTrajectory(
	measurements []*data.Measurement[float64],
	lowIndex, highIndex int,
	lowPrice, highPrice float64,
) {
	move := highPrice - lowPrice

	if move <= 0 || lowPrice <= 0 {
		return
	}

	exhaustIndex := highIndex
	prevPrice := lowPrice
	maxVelocity := 0.0

	for index := lowIndex + 1; index <= highIndex; index++ {
		price, ok := quotePrice(measurements[index])

		if !ok {
			continue
		}

		delta := price - prevPrice

		if delta > maxVelocity {
			maxVelocity = delta
		}

		progress := (price - lowPrice) / move

		if progress >= 0.50 {
			if delta <= 0 || (maxVelocity > 0 && delta < maxVelocity*0.5) {
				exhaustIndex = index
				break
			}
		}

		prevPrice = price
	}

	startPrecursor := 0

	if lowIndex > 2 {
		startPrecursor = rand.IntN(lowIndex - 1)
	}

	entryMeas := measurements[lowIndex]
	exitMeas := measurements[exhaustIndex]

	entryAsk := entryMeas.GetMetric("ask").Exact
	exitBid := exitMeas.GetMetric("bid").Exact

	if entryAsk == nil || exitBid == nil || entryAsk.Sign() <= 0 || exitBid.Sign() <= 0 {
		return
	}

	precursorTimeline := measurements[startPrecursor:lowIndex]
	holdingTimeline := measurements[lowIndex+1 : exhaustIndex+1]

	var precursorTicks []*data.Measurement[float64]
	var precursorSignals []*data.Measurement[float64]

	for _, measurement := range precursorTimeline {
		if measurement == nil {
			continue
		}

		if measurement.Source == "spot:ticker" {
			precursorTicks = append(precursorTicks, measurement)
			continue
		}

		precursorSignals = append(precursorSignals, measurement)
	}

	var holdingTicks []*data.Measurement[float64]
	var holdingSignals []*data.Measurement[float64]

	for _, measurement := range holdingTimeline {
		if measurement == nil {
			continue
		}

		if measurement.Source == "spot:ticker" {
			holdingTicks = append(holdingTicks, measurement)
			continue
		}

		holdingSignals = append(holdingSignals, measurement)
	}

	if len(precursorSignals) == 0 {
		precursorSignals = precursorTimeline
	}

	if len(holdingSignals) == 0 {
		holdingSignals = holdingTimeline
	}

	detector.queue.Enqueue(Trajectory{
		Symbol:    entryMeas.Label,
		EntryAsk:  entryAsk,
		ExitBid:   exitBid,
		Precursor: precursorSignals,
		Holding:   holdingSignals,
		Ticks:     [2][]*data.Measurement[float64]{precursorTicks, holdingTicks},
	})
}

func quotePrice(measurement *data.Measurement[float64]) (float64, bool) {
	if measurement == nil {
		return 0, false
	}

	if priceMetric, ok := measurement.LookupMetric("price"); ok && priceMetric.Raw > 0 {
		return priceMetric.Raw, true
	}

	bid, hasBid := measurement.LookupMetric("bid")
	ask, hasAsk := measurement.LookupMetric("ask")

	if hasBid && hasAsk && bid.Raw > 0 && ask.Raw > 0 {
		return bid.Exact.Add(ask.Exact).Div(decimal.NewFromInt64(2)).Float64(), true
	}

	return 0, false
}
