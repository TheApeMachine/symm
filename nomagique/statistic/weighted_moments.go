package statistic

/* WeightedMoments owns weighted Welford statistics and effective support. */
type WeightedMoments struct {
	Mass, Squared, Mean, M2 float64
}

func (moments *WeightedMoments) Update(value, weight float64) {
	if weight <= 0 {
		panic("weighted moments: weight must be positive")
	}

	moments.Mass += weight
	moments.Squared += weight * weight
	delta := value - moments.Mean
	moments.Mean += weight * delta / moments.Mass
	moments.M2 += weight * delta * (value - moments.Mean)
}

func (moments WeightedMoments) Support() float64 {
	if moments.Mass == 0 {
		return 0
	}

	return moments.Mass * moments.Mass / moments.Squared
}

func (moments *WeightedMoments) Merge(other WeightedMoments) {
	if other.Mass == 0 {
		return
	}
	if moments.Mass == 0 {
		*moments = other
		return
	}

	mass := moments.Mass + other.Mass
	delta := other.Mean - moments.Mean
	moments.M2 += other.M2 + delta*delta*moments.Mass*other.Mass/mass
	moments.Mean += delta * other.Mass / mass
	moments.Mass = mass
	moments.Squared += other.Squared
}
