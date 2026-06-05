package numeric

import "time"

/*
ShiftPriceSamples moves timestamps by offset without changing prices. Lead-lag
scoring uses this to test whether an anchor path explains a later follower path.
*/
func ShiftPriceSamples(samples []PriceSample, offset time.Duration) []PriceSample {
	return ShiftPriceSamplesInto(nil, samples, offset)
}

/*
ShiftPriceSamplesInto writes samples with timestamps moved by offset into
destination. It preserves ShiftPriceSamples semantics while allowing hot callers to
reuse storage across lag scans.
*/
func ShiftPriceSamplesInto(
	destination []PriceSample,
	samples []PriceSample,
	offset time.Duration,
) []PriceSample {
	if len(samples) == 0 {
		return destination[:0]
	}

	if cap(destination) < len(samples) {
		destination = make([]PriceSample, len(samples))
	} else {
		destination = destination[:len(samples)]
	}

	if offset == 0 {
		copy(destination, samples)

		return destination
	}

	for index := range samples {
		destination[index] = PriceSample{
			At:    samples[index].At.Add(offset),
			Price: samples[index].Price,
		}
	}

	return destination
}
