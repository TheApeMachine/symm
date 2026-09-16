package statistic

/* WeightedMoments owns weighted Welford statistics and effective support. */
type WeightedMoments struct {
	Mass, Squared, Mean, M2 float64
}

func (weightedMoments *WeightedMoments) Update(value, weight float64) {
	if weight <= 0 {
		panic("weighted moments: weight must be positive")
	}

	weightedMoments.Mass += weight
	weightedMoments.Squared += weight * weight
	delta := value - weightedMoments.Mean
	weightedMoments.Mean += weight * delta / weightedMoments.Mass
	weightedMoments.M2 += weight * delta * (value - weightedMoments.Mean)
}

func (weightedMoments WeightedMoments) Support() float64 {
	if weightedMoments.Mass == 0 {
		return 0
	}

	return weightedMoments.Mass * weightedMoments.Mass / weightedMoments.Squared
}

func (weightedMoments *WeightedMoments) Merge(other WeightedMoments) {
	if other.Mass == 0 {
		return
	}
	if weightedMoments.Mass == 0 {
		*weightedMoments = other
		return
	}

	mass := weightedMoments.Mass + other.Mass
	delta := other.Mean - weightedMoments.Mean
	weightedMoments.M2 += other.M2 + delta*delta*weightedMoments.Mass*other.Mass/mass
	weightedMoments.Mean += delta * other.Mass / mass
	weightedMoments.Mass = mass
	weightedMoments.Squared += other.Squared
}
