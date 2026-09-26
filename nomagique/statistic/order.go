package statistic

import (
	"context"
	"math"
	"sort"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
OrderServer summarises a cross section by where its observations fall.

A median and an interquartile range describe a set that a mean and a standard
deviation would misreport whenever a few members move far more than the rest.
The counts of which way members moved travel with it, because breadth and
magnitude are different questions about the same cross section.
*/
type OrderServer struct {
	*runtime.System
	reading ordered
}

/*
ordered is one cross section's order statistics.
*/
type ordered struct {
	median             float64
	lowerQuartile      float64
	upperQuartile      float64
	interquartile      float64
	medianAbsolute     float64
	extremeMagnitude   float64
	extremeSigned      float64
	extremeIndex       float64
	extremeProminence  float64
	extremeCurvature   float64
	count              float64
	positive           float64
	negative           float64
	zero               float64
	sumAbsolute        float64
	meanAbsolute       float64
	rms                float64
	medianDeviation    float64
	magnitudeDeviation float64
}

func NewOrder(ctx context.Context) *OrderServer {
	server := &OrderServer{
		System: runtime.NewSystem(ctx, "statistic.order"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write summarises the cross section that landed.
*/
func (server *OrderServer) Write(ctx context.Context, call Order_write) error {
	observed, err := call.Args().Value()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[statistic.order.Write] failed to read value argument",
			err,
		))
	}

	server.reading = summarise(observed)
	return nil
}

/*
Done reports the summary and clears it.
*/
func (server *OrderServer) Done(ctx context.Context, call Order_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.order.Done] failed to allocate results",
			err,
		))
	}

	results.SetMedian(server.reading.median)
	results.SetLowerQuartile(server.reading.lowerQuartile)
	results.SetUpperQuartile(server.reading.upperQuartile)
	results.SetInterquartile(server.reading.interquartile)
	results.SetMedianAbsolute(server.reading.medianAbsolute)
	results.SetExtremeMagnitude(server.reading.extremeMagnitude)
	results.SetExtremeSigned(server.reading.extremeSigned)
	results.SetExtremeIndex(server.reading.extremeIndex)
	results.SetExtremeProminence(server.reading.extremeProminence)
	results.SetExtremeCurvature(server.reading.extremeCurvature)
	results.SetCount(server.reading.count)
	results.SetPositive(server.reading.positive)
	results.SetNegative(server.reading.negative)
	results.SetZero(server.reading.zero)
	results.SetSumAbsolute(server.reading.sumAbsolute)
	results.SetMeanAbsolute(server.reading.meanAbsolute)
	results.SetRms(server.reading.rms)
	results.SetMedianDeviation(server.reading.medianDeviation)
	results.SetMagnitudeDeviation(server.reading.magnitudeDeviation)

	server.reading = ordered{}
	return nil
}

/*
summarise forms the order statistics of one cross section.
*/
func summarise(observed capnp.Float64List) ordered {
	if observed.Len() == 0 {
		return ordered{}
	}

	values := make([]float64, 0, observed.Len())
	magnitudes := make([]float64, 0, observed.Len())
	reading := ordered{count: float64(observed.Len())}

	for index := range observed.Len() {
		value := observed.At(index)
		values = append(values, value)
		magnitudes = append(magnitudes, math.Abs(value))
		reading.sumAbsolute += math.Abs(value)
		reading.rms += value * value

		if value > 0 {
			reading.positive++
		}

		if value < 0 {
			reading.negative++
		}

		if value == 0 {
			reading.zero++
		}

		if math.Abs(value) > reading.extremeMagnitude {
			reading.extremeMagnitude = math.Abs(value)
			reading.extremeSigned = value
			reading.extremeIndex = float64(index)
		}
	}

	describePeak(&reading, values)

	sort.Float64s(values)
	sort.Float64s(magnitudes)

	reading.median = middle(values)
	reading.medianAbsolute = middle(magnitudes)
	reading.lowerQuartile = quantile(values, 0.25)
	reading.upperQuartile = quantile(values, 0.75)
	reading.interquartile = reading.upperQuartile - reading.lowerQuartile
	reading.meanAbsolute = reading.sumAbsolute / reading.count
	reading.rms = math.Sqrt(reading.rms / reading.count)
	for index := range values {
		values[index] = math.Abs(values[index] - reading.median)
		magnitudes[index] = math.Abs(magnitudes[index] - reading.medianAbsolute)
	}
	sort.Float64s(values)
	sort.Float64s(magnitudes)
	reading.medianDeviation = middle(values)
	reading.magnitudeDeviation = middle(magnitudes)

	return reading
}

/*
middle is the central order statistic of a sorted set.
*/
func middle(sorted []float64) float64 {
	count := len(sorted)
	return (sorted[(count-1)/2] + sorted[count/2]) * 0.5
}

/*
quantile reads the value below which the given share of a sorted set falls,
interpolating between the two observations it lies among.
*/
func quantile(sorted []float64, share float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}

	position := share * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))

	if lower == upper {
		return sorted[lower]
	}

	return sorted[lower] + (position-float64(lower))*(sorted[upper]-sorted[lower])
}

/*
describePeak says how much the extreme stands out from the set and how sharply
it falls away on either side.

A search that returns its best candidate says nothing about whether that
candidate was a peak at all. Prominence separates one that towers over the
rest from one that merely edged out a flat field, and curvature separates a
sharp peak from a broad plateau where the choice was close to arbitrary.
*/
func describePeak(reading *ordered, values []float64) {
	position := int(reading.extremeIndex)

	if len(values) < 2 {
		return
	}

	rest := 0.0
	counted := 0.0

	for index, value := range values {
		if index == position {
			continue
		}

		rest += math.Abs(value)
		counted++
	}

	if counted > 0 {
		reading.extremeProminence = reading.extremeMagnitude - rest/counted
	}

	if position == 0 || position == len(values)-1 {
		return
	}

	// The second difference across the peak: how fast it falls away.
	reading.extremeCurvature = values[position-1] - 2*values[position] + values[position+1]
}
