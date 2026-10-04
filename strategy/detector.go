package strategy

import (
	"context"
	"fmt"
	"iter"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Detector scans market tape from beginning to end.
Given that we have the raw market data, directly from the spot
WebSocket, stored as *data.Measurement[float64] frames, we do
not need to do much complex math or actual "detection".
All we need to do is find the lowest and highest points on a
length of tape, provided that lowest point is before that highest
point, and pull that sub-slice out as the excursion (with a little)
extra on each side of course.
*/
type Detector struct {
	*runtime.System
	storeTee runtime.Tee
}

/*
NewDetector creates a new detector, and instantiates a queue we will
use to store our detected excursions.
*/
func NewDetector(
	ctx context.Context, storeTee runtime.Tee,
) *Detector {
	return &Detector{
		System:   runtime.NewSystem(ctx, "detector"),
		storeTee: storeTee,
	}
}

/*
Scan scans the tape once from beginning to end and keeps, per epoch and
symbol, the low/high pair with the largest relative gain where the low comes
before the high. The running trough is the only candidate low: any later high
is measured against the lowest price seen before it.
Since we store all data the system produces in its "native" format,
meaning *data.Measurement[float64], we have to realize that there is only
one Iceberg table (measurements), and we must rely on the measurement.Source
field (which is the stage that produced the measurement, like spot:trade,
correlation:trade, resonance, etc.), and the measurement.Label, which is
the symbol (meaning "ETH", "BTC", etc), to make sure we are not doing something
degenerate like mixing ticker, trade, and level3 accidentally as the tape.

NOTE: While I was here, I went ahead and moved to using an iterator, which will
greatly reduce the memory consumption of this part of the system.

NOTE: I just realized we can make this even simpler, and just get the Epoch and
SeqIdx, then store that in an Iceberg table. That basically gives us a way to
query the exact excursion, directly from the catalog, which also gives us a
clean way to do the scan for each historical tape fragment only once.
*/
func (detector *Detector) Scan(
	measurements iter.Seq[*data.Measurement[float64]],
) ([]*data.Measurement[float64], error) {
	var (
		detections    []*data.Measurement[float64]
		currentEpoch  int64
		currentSymbol string
		symbolTrades  []*data.Measurement[float64]
	)

	flushSymbol := func() {
		if len(symbolTrades) < 3 {
			return
		}

		bestLowIdx := -1
		bestHighIdx := -1
		var (
			bestPLow  *decimal.Decimal
			bestPHigh *decimal.Decimal
		)

		// Start candidate low at index 1 so that trade 0 provides precursor tape.
		runningLowIdx := 1

		for j := 2; j < len(symbolTrades); j++ {
			pLow := symbolTrades[runningLowIdx].GetMetric("price").Exact
			pHigh := symbolTrades[j].GetMetric("price").Exact

			if pLow == nil || pHigh == nil || pLow.Sign() <= 0 || pHigh.Sign() <= 0 {
				continue
			}

			if pHigh.Cmp(pLow) > 0 {
				isBetter := false

				if bestPLow == nil {
					isBetter = true
				}

				if bestPLow != nil {
					crossNew := exactProduct(pHigh, bestPLow)
					crossBest := exactProduct(bestPHigh, pLow)
					cmpVal := crossNew.Cmp(crossBest)

					if cmpVal > 0 {
						isBetter = true
					}

					if cmpVal == 0 && (j-runningLowIdx) > (bestHighIdx-bestLowIdx) {
						isBetter = true
					}
				}

				if isBetter {
					bestLowIdx = runningLowIdx
					bestHighIdx = j
					bestPLow = pLow
					bestPHigh = pHigh
				}
			}

			if pHigh.Cmp(pLow) < 0 {
				runningLowIdx = j
			}
		}

		if bestLowIdx < 1 || bestHighIdx <= bestLowIdx {
			return
		}

		low := symbolTrades[bestLowIdx]
		high := symbolTrades[bestHighIdx]
		spanTrades := bestHighIdx - bestLowIdx

		// Point A: additional tape to the start for precursor detection
		startTradeIdx := max(0, bestLowIdx-spanTrades)
		startTrade := symbolTrades[startTradeIdx]

		startTick := startTrade.Tick
		startSeqIdx := startTrade.SeqIdx

		if startSeqIdx <= 0 {
			startSeqIdx = 1
		}

		detection := detector.Flush(
			currentSymbol,
			currentEpoch,
			startSeqIdx,
			low.SeqIdx,
			high.SeqIdx,
			startTick,
			low.Tick,
			high.Tick,
			low.At,
			high.At,
			low.GetMetric("price").Exact,
			high.GetMetric("price").Exact,
		)

		if detection != nil {
			detections = append(detections, detection)
		}
	}

	for measurement := range measurements {
		if measurement.Source != "spot:trade" {
			continue
		}

		price := measurement.GetMetric("price").Exact

		if price == nil {
			return detections, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[detector] trade %d/%s/%d has no exact price",
					measurement.Epoch, measurement.Label, measurement.Tick,
				),
				nil,
			))
		}

		if currentEpoch == 0 || currentEpoch != measurement.Epoch || currentSymbol != measurement.Label {
			flushSymbol()
			currentEpoch = measurement.Epoch
			currentSymbol = measurement.Label
			symbolTrades = symbolTrades[:0]
		}

		symbolTrades = append(symbolTrades, measurement)
	}

	flushSymbol()
	return detections, nil
}

/*
exactProduct multiplies two decimals without rounding. The SDK rounds a
product to the left operand's scale, so the left operand carries both scales.
*/
func exactProduct(left, right *decimal.Decimal) *decimal.Decimal {
	return left.SetScale(left.GetScale() + right.GetScale()).Mul(right)
}

/*
Flush the detection to the storeTee to queue it up for shipping to the
Iceberg tables as a *data.Measurement[float64] shape, and return it.
*/
func (detector *Detector) Flush(
	symbol string,
	epoch int64,
	startIdx int64,
	lowIdx int64,
	highIdx int64,
	startTick int64,
	lowTick int64,
	highTick int64,
	lowAt time.Time,
	highAt time.Time,
	lowest *decimal.Decimal,
	highest *decimal.Decimal,
) *data.Measurement[float64] {
	if epoch == 0 || lowIdx == 0 || highIdx == 0 {
		return nil
	}

	metrics := map[string]data.Metric[float64]{
		"StartSeqIdx": data.NewMetric[float64](
			"start_seq_idx",
			data.UnitCount,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(float64(startIdx)),
		"LowSeqIdx": data.NewMetric[float64](
			"low_seq_idx",
			data.UnitCount,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(float64(lowIdx)),
		"HighSeqIdx": data.NewMetric[float64](
			"high_seq_idx",
			data.UnitCount,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(float64(highIdx)),
		"StartTick": data.NewMetric[float64](
			"start_tick",
			data.UnitCount,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(float64(startTick)),
		"LowTick": data.NewMetric[float64](
			"low_tick",
			data.UnitCount,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(float64(lowTick)),
		"HighTick": data.NewMetric[float64](
			"high_tick",
			data.UnitCount,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(float64(highTick)),
	}

	if lowest != nil {
		metric := data.NewMetric[float64](
			"low_price",
			data.UnitCurrency,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(lowest.Float64())
		metric.Exact = lowest
		metrics["LowPrice"] = metric
	}

	if highest != nil {
		metric := data.NewMetric[float64](
			"high_price",
			data.UnitCurrency,
			data.TimescaleInstantaneous,
			0,
			1,
		).Write(highest.Float64())
		metric.Exact = highest
		metrics["HighPrice"] = metric
	}

	measurement := data.NewMeasurement(
		detector.Name(),
		metrics,
	)

	measurement.Epoch = epoch
	measurement.Tick = highTick
	measurement.Label = symbol
	measurement.At = highAt
	measurement.From = lowAt
	measurement.Timestamp = time.Now().UnixNano()

	detector.storeTee.Push(data.NewPublication(
		measurement,
		nil,
	))

	return measurement
}
