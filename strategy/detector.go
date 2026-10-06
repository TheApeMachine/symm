package strategy

import (
	"context"
	"iter"
	"math/big"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Detector scans market tape from beginning to end.
Given raw spot trade measurements, the detector finds the single best
causal low -> high excursion for each symbol and epoch.
The scan is streaming and O(1). It keeps:

- the running trough, which is the best possible low for future highs;
- the best completed excursion observed anywhere in the tape so far.

These are separate pieces of state. A later lower wick may replace the
running trough without destroying an earlier, larger completed excursion.
*/
type Detector struct {
	*runtime.System
	storeTee runtime.Tee
	price    *broker.Price
}

/*
NewDetector creates a new detector and stores the tee used to publish
detected excursions.
*/
func NewDetector(
	ctx context.Context,
	storeTee runtime.Tee,
	price *broker.Price,
) *Detector {
	return &Detector{
		System:   runtime.NewSystem(ctx, "detector"),
		storeTee: storeTee,
		price:    price,
	}
}

/*
Scan scans the tape once from beginning to end.
For each contiguous symbol/epoch tape, the detector finds the causal
low/high pair having the largest gross price multiplier:

highPrice / lowPrice

subject to:

lowTick < highTick

The running trough is the minimum price observed before the current trade.
Every valid future high is evaluated against that trough.
Once a completed excursion becomes the best excursion seen so far, it is
retained independently of subsequent trough changes. This prevents a late
stop-loss wick from destroying an earlier macro move.
If two excursions have exactly the same gain, the wider excursion wins.
Only the winning low/high coordinates are retained. The complete native
tape fragment can later be recovered from storage using its epoch and
sequence coordinates.
*/
func (detector *Detector) Scan(
	measurements iter.Seq[*data.Measurement],
) {
	var (
		active bool

		// Active tape.
		epoch     int64
		symbol    string
		startIdx  int64
		startTick int64

		// Running trough.
		troughPrice *decimal.Decimal
		troughIdx   int64
		troughTick  int64
		troughAt    time.Time

		// Best completed excursion.
		bestGain      *big.Rat
		bestLowPrice  *decimal.Decimal
		bestLowIdx    int64
		bestLowTick   int64
		bestLowAt     time.Time
		bestHighPrice *decimal.Decimal
		bestHighIdx   int64
		bestHighTick  int64
		bestHighAt    time.Time
	)

	reset := func(measurement *data.Measurement) {
		active = true

		epoch = measurement.Epoch
		symbol = measurement.Label
		startIdx = measurement.SeqIdx
		startTick = measurement.Tick

		troughPrice = nil
		troughIdx = 0
		troughTick = 0
		troughAt = time.Time{}

		bestGain = nil

		bestLowPrice = nil
		bestLowIdx = 0
		bestLowTick = 0
		bestLowAt = time.Time{}

		bestHighPrice = nil
		bestHighIdx = 0
		bestHighTick = 0
		bestHighAt = time.Time{}
	}

	flush := func() {
		if !active {
			return
		}

		if bestLowPrice == nil || bestHighPrice == nil {
			return
		}

		if bestLowTick >= bestHighTick {
			return
		}

		if !detector.clearFriction(
			bestLowPrice,
			bestHighPrice,
			symbol,
		) {
			return
		}

		detector.Flush(
			symbol,
			epoch,
			startIdx,
			startTick,
			bestLowIdx,
			bestHighIdx,
			bestLowTick,
			bestHighTick,
			bestLowAt,
			bestHighAt,
			bestLowPrice,
			bestHighPrice,
		)
	}

	for measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Source != "spot:trade" {
			continue
		}

		// Start a new tape, or flush the completed tape when its
		// symbol/epoch boundary is crossed.
		if !active {
			reset(measurement)
		}

		if measurement.Label != symbol || measurement.Epoch != epoch {
			flush()
			reset(measurement)
		}

		price := data.Pull(measurement.Read("price")).Metric.Exact
		if price == nil || price.Sign() <= 0 {
			continue
		}

		// Maintain the running minimum.
		// Equal prices deliberately do not replace the existing trough.
		// Keeping the earliest occurrence gives the widest interval when
		// the same low price occurs multiple times.
		if troughPrice == nil || price.Cmp(troughPrice) < 0 {
			troughPrice = price
			troughIdx = measurement.SeqIdx
			troughTick = measurement.Tick
			troughAt = measurement.At
			continue
		}

		// A valid excursion must be causal and must actually rise from
		// the current trough.
		if measurement.Tick <= troughTick {
			continue
		}

		if price.Cmp(troughPrice) <= 0 {
			continue
		}

		// Gross return ratio for the candidate excursion.
		// Rational representation preserves exact arithmetic precision
		// without truncation or scale artifacts.
		gain := new(big.Rat).Quo(price.Rat(), troughPrice.Rat())

		if bestGain != nil {
			cmp := gain.Cmp(bestGain)

			// Strictly worse than the best completed excursion.
			if cmp < 0 {
				continue
			}

			// For identical gains, retain the widest tape fragment.
			if cmp == 0 {
				candidateSpan := measurement.Tick - troughTick
				bestSpan := bestHighTick - bestLowTick

				if candidateSpan <= bestSpan {
					continue
				}
			}
		}

		// This trough -> current price pair is now the globally best
		// completed excursion observed in this symbol/epoch.
		// Crucially, these values are independent from the running
		// trough after being recorded. A later lower wick cannot erase
		// this excursion unless a subsequent rally actually beats it.
		bestGain = gain

		bestLowPrice = troughPrice
		bestLowIdx = troughIdx
		bestLowTick = troughTick
		bestLowAt = troughAt

		bestHighPrice = price
		bestHighIdx = measurement.SeqIdx
		bestHighTick = measurement.Tick
		bestHighAt = measurement.At
	}

	// Flush the final active tape after the iterator is exhausted.
	flush()
}

/*
clearFriction verifies that the selected excursion remains profitable after
round-trip taker friction.
*/
func (detector *Detector) clearFriction(
	lowPrice *decimal.Decimal,
	highPrice *decimal.Decimal,
	symbol string,
) bool {
	if lowPrice == nil ||
		highPrice == nil ||
		lowPrice.Sign() <= 0 ||
		highPrice.Sign() <= 0 {
		return false
	}

	if detector.price == nil {
		return highPrice.Cmp(lowPrice) > 0
	}

	pnl, _, err := detector.price.RoundTrip(
		symbol,
		lowPrice,
		highPrice,
	)

	if err != nil || pnl == nil {
		return false
	}

	return pnl.Sign() > 0
}

/*
Flush writes the detected excursion to storeTee.
The stored epoch and sequence coordinates identify the exact contiguous
historical tape fragment from the winning low through the winning high.
*/
func (detector *Detector) Flush(
	symbol string,
	epoch int64,
	startIdx int64,
	startTick int64,
	lowIdx int64,
	highIdx int64,
	lowTick int64,
	highTick int64,
	lowAt time.Time,
	highAt time.Time,
	lowPrice *decimal.Decimal,
	highPrice *decimal.Decimal,
) *data.Measurement {
	measurement := data.NewMeasurement(
		epoch,
		symbol,
		detector.Name(),
		startIdx,
		highTick,
	)
	measurement.At = highAt
	measurement.From = lowAt

	measurement.Write(
		data.NewMetric("start_idx", float64(startIdx), data.UnitCount, data.TimescaleTick),
		data.NewMetric("start_tick", float64(startTick), data.UnitCount, data.TimescaleTick),
		data.NewMetric("low_idx", float64(lowIdx), data.UnitCount, data.TimescaleTick),
		data.NewMetric("low_tick", float64(lowTick), data.UnitCount, data.TimescaleTick),
		data.NewMetric("high_idx", float64(highIdx), data.UnitCount, data.TimescaleTick),
		data.NewMetric("high_tick", float64(highTick), data.UnitCount, data.TimescaleTick),
		data.NewExactMetric("low_price", lowPrice, data.UnitPrice, data.TimescaleTick),
		data.NewExactMetric("high_price", highPrice, data.UnitPrice, data.TimescaleTick),
	)

	detector.storeTee.Push(
		data.NewPublication(
			measurement,
			nil,
		),
	)

	return measurement
}
