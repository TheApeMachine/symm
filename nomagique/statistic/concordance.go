package statistic

import "math"

/*
Concordance retains sufficient statistics of paired, dimensionless movements.
An inactive side contributes zero alignment, including an absent observation.
Both inactive sides supply no evidence. A consistent inverse pair has the same
strength as a consistent direct pair, but the opposite Orientation.
*/
type Concordance struct {
	Support       float64
	Aligned       float64
	WeightSquared float64
	Mean          float64
	M2            float64
	Magnitude     float64
}

type ConcordanceReading struct {
	Strength    float64
	Orientation float64
	Support     float64
}

/*
Update weights evidence by the supplied clock increment. The attraction is
absolute mean sign agreement minus its standard error, plus relative magnitude
agreement supported by that sign agreement. Thus uncertain or contradictory
alignment can repel; no selected correlation cutoff is used.
*/
func (statistic *Concordance) Update(left, right, weight float64) ConcordanceReading {
	prior := statistic.Aligned
	alignment := 0.0
	active := left != 0 || right != 0

	if weight < 0 {
		panic("concordance: negative clock increment")
	}

	if weight != 0 && active {

		if left != 0 && right != 0 {
			alignment = math.Copysign(1, left) * math.Copysign(1, right)
		}

		statistic.Support += weight
		statistic.Aligned += weight * alignment
		statistic.WeightSquared += weight * weight
		delta := alignment - statistic.Mean
		statistic.Mean += weight * delta / statistic.Support
		statistic.M2 += weight * delta * (alignment - statistic.Mean)
		denominator := math.Abs(left) + math.Abs(right)
		statistic.Magnitude += weight * (1 - math.Abs(math.Abs(left)-math.Abs(right))/denominator)
	}

	reading := statistic.Reading()

	if weight != 0 && active && prior != 0 && alignment != math.Copysign(1, prior) {
		reading.Strength = -math.Abs(reading.Strength)
	}

	return reading
}

func (statistic *Concordance) Reading() ConcordanceReading {
	reading := ConcordanceReading{Support: statistic.Support}

	if statistic.Support == 0 {
		return reading
	}

	mean := statistic.Aligned / statistic.Support
	reading.Orientation = math.Copysign(1, mean)
	consistency := math.Abs(mean)
	// E[(sign-mean)^2] includes unilateral movement as disagreement.
	effective := statistic.Support * statistic.Support / statistic.WeightSquared
	variance := statistic.M2 / statistic.Support
	uncertainty := math.Sqrt(variance / effective)
	reading.Strength = consistency - uncertainty + consistency*statistic.Magnitude/statistic.Support
	return reading
}
