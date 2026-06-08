package numeric

import "time"

/*
ShiftPriceSamplesInto writes samples with timestamps moved by offset into
destination. Lead-lag scoring reuses caller storage across lag scans.
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
