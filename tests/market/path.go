package market

/* Trend walks a test path by the declared relative step; rates are fixture inputs. */
func Trend(from float64, steps int, rate float64) []float64 {
	path := make([]float64, 0, steps)
	value := from
	for range steps {
		value *= 1 + rate
		path = append(path, value)
	}
	return path
}

/* Reversal traverses a rise, decline and recovery with deliberately spaced feed stamps. */
func Reversal() []float64 {
	path := Trend(100, 60, 0.005)
	path = append(path, Trend(path[len(path)-1], 80, -0.005)...)
	return append(path, Trend(path[len(path)-1], 80, 0.005)...)
}
