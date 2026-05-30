package numeric

type Select struct {
	selectValues func(out float64, values []float64) []float64
}

func NewSelect(fn func(out float64, values []float64) []float64) *Select {
	return &Select{selectValues: fn}
}

func (selectStage *Select) Next(out float64, values ...float64) (float64, error) {
	selected := selectStage.selectValues(out, values)

	if len(selected) == 0 {
		return out, nil
	}

	if len(selected) == 1 {
		return selected[0], nil
	}

	// Product-style fallback keeps this useful as a tiny fan-in primitive.
	result := 1.0
	for _, value := range selected {
		if value <= 0 {
			return 0, nil
		}

		result *= value
	}

	return result, nil
}

func (selectStage *Select) Reset() error {
	return nil
}
