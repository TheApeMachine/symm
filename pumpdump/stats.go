package pumpdump

import "math"

func quartiles(values []float64) (lower, upper float64) {
	if len(values) == 0 {
		return 0, 0
	}

	cp := append([]float64(nil), values...)
	sortFloats(cp)

	lower = percentileSorted(cp, 0.25)
	upper = percentileSorted(cp, 0.75)

	return lower, upper
}

func percentileSorted(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if quantile <= 0 {
		return sorted[0]
	}

	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}

	position := quantile * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(position))
	upperIndex := int(math.Ceil(position))
	weight := position - float64(lowerIndex)

	return sorted[lowerIndex]*(1-weight) + sorted[upperIndex]*weight
}

func volumeRatioFence(ratios []float64) float64 {
	if len(ratios) == 0 {
		return 0
	}

	lower, upper := quartiles(ratios)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return maxFloat(ratios)
}

func volumeRatios(volumes []float64) []float64 {
	baseline := mean(volumes)

	if baseline <= 0 {
		return nil
	}

	ratios := make([]float64, len(volumes))

	for index, volume := range volumes {
		ratios[index] = volume / baseline
	}

	return ratios
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	peak := values[0]

	for _, value := range values[1:] {
		if value > peak {
			peak = value
		}
	}

	return peak
}

func crossSectionMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	cp := append([]float64(nil), values...)
	sortFloats(cp)

	return median(cp)
}
