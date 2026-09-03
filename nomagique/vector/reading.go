package vector

/*
Reading is the unpacked result of one classification: which class won, how
confidently, how ambiguous the distribution was, and the mass by class label.
*/
type Reading struct {
	Ready         bool
	WinnerIndex   int
	WinnerLabel   string
	Confidence    float64
	Ambiguity     float64
	Sharpness     float64
	Maturity      float64
	Probabilities map[string]float64
}

/*
Read unpacks the current distribution into a labelled reading. An unready
distribution yields an unready Reading, so an absent classification is never
mistaken for a uniform one.
*/
func (classifier *Classifier) Read() Reading {
	distribution := classifier.Distribution()

	if !distribution.Ready() {
		return Reading{}
	}

	groups := classifier.Groups()

	reading := Reading{
		Ready:         true,
		WinnerIndex:   distribution.Winner(),
		Confidence:    float64(distribution.Confidence()),
		Ambiguity:     float64(distribution.Ambiguity()),
		Sharpness:     float64(distribution.Sharpness()),
		Maturity:      float64(classifier.Maturity()),
		Probabilities: make(map[string]float64, len(groups)),
	}

	if reading.WinnerIndex >= 0 && reading.WinnerIndex < len(groups) {
		reading.WinnerLabel = groups[reading.WinnerIndex].Label
	}

	for index, group := range groups {
		reading.Probabilities[group.Label] = float64(distribution.Probability(index))
	}

	return reading
}

/*
Probability returns the mass assigned to one class label.
*/
func (reading Reading) Probability(label string) (float64, bool) {
	value, found := reading.Probabilities[label]

	return value, found
}
