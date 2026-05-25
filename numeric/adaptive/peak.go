package adaptive

/*
Peak passes out through when it is greater than or equal to every peer value.
Zero peers accepts any positive out.
*/
type Peak struct{}

/*
NewPeak creates a cross-section peak gate dynamic.
*/
func NewPeak() *Peak {
	return &Peak{}
}

/*
Next returns out when out is the peak among out and values, otherwise zero.
*/
func (peak *Peak) Next(out float64, values ...float64) (float64, error) {
	if out <= 0 {
		return 0, nil
	}

	for _, peer := range values {
		if peer > out {
			return 0, nil
		}
	}

	return out, nil
}

/*
Reset is a no-op for Peak.
*/
func (peak *Peak) Reset() error {
	return nil
}
