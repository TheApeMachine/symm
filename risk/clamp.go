package risk

func clampUnit(value float64) float64 {
	if value <= 0 {
		return 0
	}

	if value >= 1 {
		return 1
	}

	return value
}
