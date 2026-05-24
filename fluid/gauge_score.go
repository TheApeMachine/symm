package fluid

import "math"

/*
fieldGaugeScore maps cross-section field activity into a unit-scale gauge reading.
*/
func fieldGaugeScore(field FieldAggregate) float64 {
	peak := 0.0

	for _, value := range []float64{
		field.Re,
		field.Turb,
		field.Vort,
		math.Abs(field.Div),
		field.Visc,
	} {
		if value > peak {
			peak = value
		}
	}

	if peak <= 0 {
		return 0
	}

	return math.Min(1, peak/3)
}
