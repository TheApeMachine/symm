package strategy

import (
	"context"
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
Scan scans the tape once from beginning to end, tracking the lowest point
and the highest point where the lowest is first and the highest is second.
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
) {
	var (
		epoch    int64
		symbol   string
		lowest   *decimal.Decimal
		highest  *decimal.Decimal
		lowIdx   int64
		highIdx  int64
		lowTick  int64
		highTick int64
		lowAt    time.Time
		highAt   time.Time
	)

	for measurement := range measurements {
		if measurement.Source != "spot:trade" {
			errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"",
				nil,
			))

			continue
		}

		if epoch == 0 || epoch != measurement.Epoch || symbol != measurement.Label {
			detector.Flush(symbol, epoch, lowIdx, highIdx, lowTick, highTick, lowAt, highAt, lowest, highest)
			epoch = measurement.Epoch
			symbol = measurement.Label
			lowest = nil
			highest = nil
			lowIdx = 0
			highIdx = 0
			lowTick = 0
			highTick = 0
			lowAt = time.Time{}
			highAt = time.Time{}
		}

		price := measurement.GetMetric("price").Exact

		if lowest == nil || price.Cmp(lowest) < 0 {
			lowest = price
			lowIdx = measurement.SeqIdx
			lowTick = measurement.Tick
			lowAt = measurement.At
			highest = nil
			highIdx = 0
			highTick = 0
			highAt = time.Time{}
			continue
		}

		if highest == nil || price.Cmp(highest) > 0 {
			highest = price
			highIdx = measurement.SeqIdx
			highTick = measurement.Tick
			highAt = measurement.At
		}
	}

	detector.Flush(symbol, epoch, lowIdx, highIdx, lowTick, highTick, lowAt, highAt, lowest, highest)
}

/*
Flush the detection to the storeTee to queue it up for shipping to the
Iceberg tables as a *data.Measurement[float64] shape.
*/
func (detector *Detector) Flush(
	symbol string,
	epoch int64,
	lowIdx int64,
	highIdx int64,
	lowTick int64,
	highTick int64,
	lowAt time.Time,
	highAt time.Time,
	lowest *decimal.Decimal,
	highest *decimal.Decimal,
) {
	if epoch == 0 || lowIdx == 0 || highIdx == 0 {
		return
	}

	metrics := map[string]data.Metric[float64]{
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
}
